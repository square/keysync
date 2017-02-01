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

package main

import "fmt"
import (
	"path/filepath"

	"io/ioutil"
	"os"
	"sync"

	"net/url"
	"time"

	"github.com/square/go-sq-metrics"
	klog "github.com/square/keywhiz-fs/log"
)

type syncerEntry struct {
	Client
	ClientConfig
	WriteConfig
}

// A Syncer manages a collection of clients, handling downloads and writing out updated secrets.
// Construct one using the NewSyncer and AddClient functions
type Syncer struct {
	config    *Config
	server    *url.URL
	clients   map[string]syncerEntry
	syncMutex sync.Mutex
}

// NewSyncer instantiates the main stateful object in Keysync.
func NewSyncer(config *Config, metricsHandle *sqmetrics.SquareMetrics) *Syncer {
	syncer := Syncer{config: config, clients: map[string]syncerEntry{}}
	return &syncer
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
				// TODO: Replace the background async buildConfig() with a sync one here.
				continue
			}
		}
		// Otherwise we (re)create the client
		s.clients[name] = s.buildClient(name, clientConfig)

	}
	for name, client := range s.clients {
		// TODO: Do some clean-up?
		_, ok := newConfigs[name]
		if !ok {
			fmt.Printf("Client gone: %s (%v)", name, client)
		}
	}
	return nil
}

// buildClient collects the configuration and builds a client.  Most of this code should probably be refactored ito NewClient
func (s *Syncer) buildClient(name string, clientConfig ClientConfig) syncerEntry {
	klogConfig := klog.Config{
		Debug:      s.config.Debug,
		Syslog:     false,
		Mountpoint: name,
	}
	client := NewClient(clientConfig.Cert, clientConfig.Key, s.config.CaFile, s.server, time.Minute, klogConfig, nil)
	user := clientConfig.User
	group := clientConfig.Group
	if user == "" {
		user = s.config.DefaultUser
	}
	if group == "" {
		group = s.config.DefaultGroup
	}
	writeConfig := WriteConfig{EnforceFilesystem: s.config.FsType, WritePermissions: false, DefaultOwner: NewOwnership(user, group)}
	return syncerEntry{Client: client, ClientConfig: clientConfig, WriteConfig: writeConfig}
}

// RunNow runs the syncer once, for all clients, without sleeps.
func (s *Syncer) RunNow() error {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()
	for name, entry := range s.clients {
		fmt.Printf("Updating %s", name)
		client := entry.Client
		secrets, ok := client.SecretList()
		if !ok {
			//SecretList logged the error, continue on
			continue
		}
		secretsWritten := map[string]struct{}{}
		for _, secretMetadata := range secrets {
			// TODO: Optimizations to avoid needlessly fetching secrets
			secret, err := client.Secret(secretMetadata.Name)
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
				fmt.Printf("Couldn't write secret %s: %+v\n", secret.Name, err)
				continue
			}
			secretsWritten[secret.Name] = struct{}{}
		}
		fileInfos, err := ioutil.ReadDir(entry.Mountpoint)
		if err != nil {
			fmt.Printf("Couldn't read directory: %s\n", entry.Mountpoint)
			continue
		}
		for _, fileInfo := range fileInfos {
			filename := fileInfo.Name()
			_, ok := secretsWritten[filename]
			if !ok {
				// This file wasn't written in the loop above, so we remove it.
				fmt.Printf("Removing %s\n", filename)
				os.Remove(filepath.Join(entry.Mountpoint, filename))
			}
		}
	}
	return nil
}
