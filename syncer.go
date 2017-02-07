// Copyright 2016 Square Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keysync

import "fmt"
import (
	"path/filepath"

	"io/ioutil"
	"os"
	"sync"

	"net/url"
	"time"

	"math/rand"

	"github.com/Sirupsen/logrus"
	"github.com/square/go-sq-metrics"
)

type syncerEntry struct {
	Client
	ClientConfig
	WriteConfig
}

// A Syncer manages a collection of clients, handling downloads and writing out updated secrets.
// Construct one using the NewSyncer and AddClient functions
type Syncer struct {
	config        *Config
	server        *url.URL
	clients       map[string]syncerEntry
	logger        *logrus.Entry
	metricsHandle *sqmetrics.SquareMetrics
	syncMutex     sync.Mutex
}

// NewSyncer instantiates the main stateful object in Keysync.
func NewSyncer(config *Config, logger *logrus.Entry, metricsHandle *sqmetrics.SquareMetrics) (*Syncer, error) {
	syncer := Syncer{config: config, clients: map[string]syncerEntry{}, logger: logger, metricsHandle: metricsHandle}
	url, err := url.Parse("https://" + config.Server)
	if err != nil {
		return nil, fmt.Errorf("Parsing server: %s", config.Server)
	}
	syncer.server = url
	return &syncer, nil
}

// LoadClients gets configured clients,
func (s *Syncer) LoadClients() error {
	newConfigs, err := s.config.LoadClients()
	if err != nil {
		return err
	}
	for name, clientConfig := range newConfigs {
		// If there's already a client loaded, reload it
		syncerEntry, ok := s.clients[name]
		if ok {
			if syncerEntry.ClientConfig == clientConfig {
				// Exists, and the same config.
				syncerEntry.Client.RebuildClient()
				continue
			}
		}
		// Otherwise we (re)create the client
		client, err := s.buildClient(name, clientConfig, s.metricsHandle)
		if err != nil {
			s.logger.WithError(err).WithField("client", name).Error("Building client")
			continue

		}
		s.clients[name] = *client
	}
	for name, client := range s.clients {
		// TODO: Record for cleanup. We don't want to actually do it in this function, so we record it for the
		// next sync call to take care of it.
		_, ok := newConfigs[name]
		if !ok {
			s.logger.Warnf("Client gone: %s (%v)", name, client)
		}
	}
	return nil
}

// buildClient collects the configuration and builds a client.  Most of this code should probably be refactored ito NewClient
func (s *Syncer) buildClient(name string, clientConfig ClientConfig, metricsHandle *sqmetrics.SquareMetrics) (*syncerEntry, error) {
	clientLogger := s.logger.WithField("client", name)
	client, err := NewClient(clientConfig.Cert, clientConfig.Key, s.config.CaFile, s.server, time.Minute, clientLogger, metricsHandle)
	if err != nil {
		return nil, err
	}
	user := clientConfig.User
	group := clientConfig.Group
	if user == "" {
		user = s.config.DefaultUser
	}
	if group == "" {
		group = s.config.DefaultGroup
	}
	defaultOwnership, err := NewOwnership(user, group)
	if err != nil {
		// We log an error here but continue on.  The default of "0", root, is safe.
		s.logger.WithError(err).Error("Default ownership")
	}
	writeConfig := WriteConfig{EnforceFilesystem: s.config.FsType, ChownFiles: s.config.ChownFiles, DefaultOwnership: defaultOwnership}
	return &syncerEntry{Client: client, ClientConfig: clientConfig, WriteConfig: writeConfig}, nil
}

// Randomize the sleep interval, increasing up to 1/4 of the duration.
func randomize(d time.Duration) time.Duration {
	maxAdded := float64(d) / 4
	amount := rand.Float64() * maxAdded

	return time.Duration(float64(d) + amount)
}

// Run the main sync loop.
func (s *Syncer) Run() error {
	pollInterval, err := time.ParseDuration(s.config.PollInterval)
	if s.config.PollInterval != "" && err != nil {
		return fmt.Errorf("Couldn't parse Poll Interval '%s': %v\n", s.config.PollInterval, err)
	}

	for {
		err = s.RunOnce()
		if err != nil {
			s.logger.WithError(err).Error("Running sync")
		}

		// No poll interval configured, so return now
		if s.config.PollInterval == "" {
			return err
		}

		time.Sleep(randomize(pollInterval))
	}
}

// RunOnce runs the syncer once, for all clients, without sleeps.
func (s *Syncer) RunOnce() error {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()
	err := s.LoadClients()
	if err != nil {
		return err
	}
	for name, entry := range s.clients {
		err = entry.Sync()
		if err != nil {
			// Record error but continue updating other clients
			s.logger.WithError(err).WithField("name", name).Error("Syncing")
		}
	}
	return nil
}

// Sync this: Download and write all secrets.
func (entry *syncerEntry) Sync() error {
	err := os.MkdirAll(entry.Mountpoint, 0775)
	if err != nil {
		return fmt.Errorf("Mkdir mountpoint '%s': %v", entry.Mountpoint, err)
	}
	secrets, ok := entry.Client.SecretList()
	if !ok {
		//SecretList logged the error, continue on
		return nil
	}
	secretsWritten := map[string]struct{}{}
	for _, secretMetadata := range secrets {
		// TODO: Optimizations to avoid needlessly fetching secrets
		secret, err := entry.Client.Secret(secretMetadata.Name)
		if err != nil {
			// client.Secret logged the error, continue on
			continue
		}
		// We split out the filename to prevent a maliciously-named secret from
		// writing outside of the intended secrets directory.
		_, filename := filepath.Split(secret.Name)
		name := filepath.Join(entry.Mountpoint, filename)
		err = atomicWrite(name, secret, entry.WriteConfig)
		if err != nil {
			entry.logger.WithError(err).WithField("file", secret.Name).Error("Writing secret")
			continue
		}
		secretsWritten[secret.Name] = struct{}{}
	}
	fileInfos, err := ioutil.ReadDir(entry.Mountpoint)
	if err != nil {
		return fmt.Errorf("Couldn't read directory: %s\n", entry.Mountpoint)
	}
	for _, fileInfo := range fileInfos {
		filename := fileInfo.Name()
		_, ok := secretsWritten[filename]
		if !ok {
			// This file wasn't written in the loop above, so we remove it.
			entry.logger.WithField("file", filename).Info("Removing old secret")
			os.Remove(filepath.Join(entry.Mountpoint, filename))
		}
	}
	return nil
}
