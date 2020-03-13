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

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/square/keysync/output"

	"github.com/sirupsen/logrus"
	sqmetrics "github.com/square/go-sq-metrics"
)

var (
	nilError error
)

// secretState records the state of a secret we've written
type secretState struct {
	// ContentHash is a Sha256 of what we wrote out, used to detect content corruption in the filesystem
	ContentHash [sha256.Size]byte
	// Checksum is the server's identifier for the contents of the hash (it's an HMAC)
	Checksum string
	// We store the mode we wrote to the filesystem
	output.FileInfo
	// Owner, Group, and Mode come from the Keywhiz server
	Owner string
	Group string
	Mode  string
}

type syncerEntry struct {
	Client
	ClientConfig
	output    Output
	SyncState map[string]secretState
}

// A Syncer manages a collection of clients, handling downloads and writing out updated secrets.
// Construct one using the NewSyncer and AddClient functions
type Syncer struct {
	config                 *Config
	server                 *url.URL
	clients                map[string]syncerEntry
	logger                 *logrus.Entry
	metricsHandle          *sqmetrics.SquareMetrics
	syncMutex              sync.Mutex
	pollInterval           time.Duration
	lastSuccessMu          sync.Mutex
	lastSuccessAt          time.Time
	startedMu              sync.Mutex
	startedAt              time.Time
	lastError              unsafe.Pointer
	disableClientReloading bool
	outputCollection       OutputCollection
}

// Updated secrets during a sync.  How many secrets were added, changed, or deleted this sync.
type Updated struct {
	Added   uint
	Changed uint
	Deleted uint
}

// Add in another update count
func (u *Updated) Add(rhs Updated) {
	u.Added += rhs.Added
	u.Changed += rhs.Changed
	u.Deleted += rhs.Changed
}

// Total of changed secrets
func (u *Updated) Total() uint {
	return u.Added + u.Changed + u.Deleted
}

// NewSyncer instantiates the main stateful object in Keysync.
func NewSyncer(config *Config, outputCollection OutputCollection, logger *logrus.Entry, metricsHandle *sqmetrics.SquareMetrics) (*Syncer, error) {
	// Pre-parse poll interval
	pollInterval := time.Duration(0)
	if config.PollInterval != "" {
		var err error
		pollInterval, err = time.ParseDuration(config.PollInterval)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse Poll Interval '%s': %v", config.PollInterval, err)
		}
		logger.Infof("Poll interval is %s", config.PollInterval)
	}

	syncer := Syncer{
		config:           config,
		clients:          map[string]syncerEntry{},
		logger:           logger,
		metricsHandle:    metricsHandle,
		pollInterval:     pollInterval,
		outputCollection: outputCollection,
	}

	serverURL, err := url.Parse("https://" + config.Server)
	if err != nil {
		return nil, fmt.Errorf("failed parsing server: %s", config.Server)
	}
	syncer.server = serverURL

	// Add callback for last success gauge
	metricsHandle.AddGauge("seconds_since_last_success", func() int64 {
		since, _ := syncer.timeSinceLastSuccess()
		return int64(since / time.Second)
	})

	syncer.updateMostRecentError(nilError)
	return &syncer, nil
}

// NewSyncerFromFile instantiates a syncer that reads from a file/bundle instead of an HTTP server.
func NewSyncerFromFile(config *Config, clientConfig ClientConfig, bundle string, logger *logrus.Entry, metricsHandle *sqmetrics.SquareMetrics) (*Syncer, error) {
	syncer := Syncer{
		config:                 config,
		clients:                map[string]syncerEntry{},
		logger:                 logger,
		metricsHandle:          metricsHandle,
		disableClientReloading: true,
	}

	client, err := NewBackupBundleClient(bundle, logger)
	if err != nil {
		return nil, err
	}

	output, err := OutputDirCollection{Config: config}.NewOutput(clientConfig, logger)
	if err != nil {
		return nil, err
	}

	syncer.clients[clientConfig.DirName] = syncerEntry{
		client,
		clientConfig,
		output,
		map[string]secretState{},
	}

	syncer.updateMostRecentError(nilError)

	return &syncer, nil
}

func (s *Syncer) updateSuccessTimestamp() {
	s.lastSuccessMu.Lock()
	defer s.lastSuccessMu.Unlock()

	s.lastSuccessAt = time.Now()
}

func (s *Syncer) updateMostRecentError(err error) {
	atomic.StorePointer(&s.lastError, unsafe.Pointer(&err))
}

// Returns time since last success. Boolean value indicates if since
// duration is valid, i.e. if keysync has succeeded at least once.
func (s *Syncer) timeSinceLastSuccess() (since time.Duration, ok bool) {
	s.lastSuccessMu.Lock()
	defer s.lastSuccessMu.Unlock()

	if s.lastSuccessAt.IsZero() {
		return 0, false
	}

	return time.Since(s.lastSuccessAt), true
}

// Returns the most recent error that was encountered. Returns nil if
// no error has been encountered, or if syncer has never been run.
func (s *Syncer) mostRecentError() (err error) {
	return *((*error)(atomic.LoadPointer(&s.lastError)))
}

type pendingCleanup struct {
	Outputs map[string]Output
}

func (p *pendingCleanup) cleanup(logger *logrus.Entry) (uint, []error) {
	var deleted uint
	var errors []error
	if p == nil {
		return deleted, errors
	}

	for name, output := range p.Outputs {
		outputDeleted, err := output.RemoveAll()
		if err != nil {
			errors = append(errors, err)
			logger.WithError(err).WithField("name", name).Warn("Failed to remove old client")
		} else {
			logger.WithField("name", name).Info("Removed old client")
			deleted += outputDeleted
		}
	}

	return deleted, errors
}

// LoadClients gets configured clients,
// This function returns clients that have been deconfigured, which are expected to be cleaned up
func (s *Syncer) LoadClients() (*pendingCleanup, error) {
	if s.disableClientReloading {
		return nil, nil
	}

	newConfigs, err := s.config.LoadClients()
	if err != nil {
		return nil, err
	}
	s.logger.WithField("count", len(newConfigs)).Info("Loaded configs")

	for name, clientConfig := range newConfigs {
		// If there's already a client loaded, reload it
		syncerEntry, ok := s.clients[name]
		if ok {
			if syncerEntry.ClientConfig == clientConfig {
				// Exists, and the same config.
				err := syncerEntry.Client.RebuildClient()
				if err != nil {
					s.logger.WithError(err).Warnf("Unable to rebuild client")
				}
				continue
			}
		}
		// Otherwise we (re)create the client
		client, err := s.buildClient(name, clientConfig, s.metricsHandle)
		if err != nil {
			s.logger.WithError(err).WithField("client", name).Error("Failed building client")
			continue

		}
		s.clients[name] = *client
	}

	pending := &pendingCleanup{Outputs: map[string]Output{}}
	for name, client := range s.clients {
		// Record which clients have gone away, for later cleanup.
		_, ok := newConfigs[name]
		if !ok {
			pending.Outputs[name] = client.output
			delete(s.clients, name)
		}
	}
	return pending, nil
}

// buildClient collects the configuration and builds a client.  Most of this code should probably be refactored ito NewClient
func (s *Syncer) buildClient(name string, clientConfig ClientConfig, metricsHandle *sqmetrics.SquareMetrics) (*syncerEntry, error) {
	clientLogger := s.logger.WithField("client", name)
	client, err := NewClient(&clientConfig, s.config.CaFile, s.server, clientLogger, metricsHandle)
	if err != nil {
		return nil, err
	}

	output, err := s.outputCollection.NewOutput(clientConfig, clientLogger)
	if err != nil {
		return nil, err
	}

	return &syncerEntry{client, clientConfig, output, map[string]secretState{}}, nil
}

// Randomize the sleep interval, increasing up to 1/4 of the duration.
func randomize(d time.Duration) time.Duration {
	maxAdded := float64(d) / 4
	amount := rand.Float64() * maxAdded

	return time.Duration(float64(d) + amount)
}

// Run the main sync loop.
func (s *Syncer) Run() error {
	s.startedMu.Lock()
	s.startedAt = time.Now()
	s.startedMu.Unlock()

	for {
		_, errors := s.RunOnce()
		var err error
		if len(errors) != 0 {
			if len(errors) == 1 {
				err = errors[0]
			} else {
				err = fmt.Errorf("errors: %v", errors)
			}
			s.logger.WithError(err).Error("Failed running sync")
		} else {
			s.logger.Debug("Updating success timestamp")
			s.updateSuccessTimestamp()
		}

		// No poll interval configured, so return now
		if s.pollInterval == 0 {
			s.logger.Info("No poll configured")
			return err
		}
		sleep := randomize(s.pollInterval)
		s.logger.WithField("duration", sleep).Info("Sleeping")
		time.Sleep(sleep)
	}
}

// RunOnce runs the syncer once, for all clients, without sleeps.
func (s *Syncer) RunOnce() (Updated, []error) {
	var updated Updated
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()
	var errors []error
	pendingCleanup, err := s.LoadClients()
	if err != nil {
		return updated, []error{err}
	}
	// Record client directories so we know what's valid in the deletion loop below
	clientDirs := map[string]struct{}{}
	for name, entry := range s.clients {
		clientDirs[entry.ClientConfig.DirName] = struct{}{}
		thisupdated, err := entry.Sync()
		if err != nil {
			// Record error but continue updating other clients
			s.logger.WithError(err).WithField("name", name).Error("Failed while syncing")
			errors = append(errors, err)
		}
		updated.Add(thisupdated)
	}

	// Remove clients that we noticed the configs disappear for.
	// While the function below would take care of it too, we don't warn in the expected case.
	deleted, errs := pendingCleanup.cleanup(s.logger)
	updated.Deleted += deleted
	errors = append(errors, errs...)

	// Clean up any old content in the secrets directory
	deleted, errs = s.outputCollection.Cleanup(clientDirs, s.logger)
	updated.Deleted += deleted
	errors = append(errors, errs...)

	s.logger.WithFields(logrus.Fields{
		"Added":   updated.Added,
		"Changed": updated.Changed,
		"Deleted": updated.Deleted,
	}).Info("Sync complete")

	return updated, errors
}

// Uptime returns the time duration since syncer was started or 0 if it's not running
func (s *Syncer) Uptime() time.Duration {
	s.startedMu.Lock()
	defer s.startedMu.Unlock()

	if s.startedAt.IsZero() {
		return 0
	}

	return time.Since(s.startedAt)
}

// Sync this: Download and write all secrets.
// Returns the number of secrets added, changed, or deleted secrets
func (entry *syncerEntry) Sync() (Updated, error) {
	updated := Updated{}

	secrets, err := entry.Client.SecretList()
	if err != nil {
		entry.Logger().WithError(err).Error("Failed to list secrets")
		return updated, err
	}

	var pendingDeletions []string
	for filename, secretMetadata := range secrets {
		if state, present := entry.SyncState[filename]; present {
			if entry.output.Validate(&secretMetadata, state) {
				// The secret is already downloaded, so no action needed
				entry.Logger().WithField("secret", secretMetadata.Name).Debug("Not requesting still-valid secret")
				continue
			}
		}
		secret, err := entry.Client.Secret(secretMetadata.Name)
		if err != nil {
			// This is essentially a race condition: A secret was deleted between listing and fetching
			if _, deleted := err.(SecretDeleted); deleted {
				// We defer actual deletion to the loop below, so that new secrets are always written
				// before any are deleted.
				pendingDeletions = append(pendingDeletions, filename)
			}
			continue
		}
		state, err := entry.output.Write(secret)
		// TODO: Filename changes of secrets might be noisy.  We should ensure they're handled more gracefully.
		if err != nil {
			entry.Logger().WithError(err).WithField("secret", secret.Name).Error("Failed while writing secret")
			// This situation is unlikely: We couldn't write the secret to disk.
			// If Output.Write fails, then no changes to the secret on-disk were made, thus we make no change
			// to the entry.SyncState
			continue
		}

		// Success!  Store the state we wrote to disk for later validation.
		entry.Logger().WithField("file", filename).Info("Wrote file")
		entry.SyncState[filename] = *state

		// Validate that we wrote our output.  This should never fail, unless there are bugs or something interfering
		// with Keysync's output files.  It is only here to help detect problems.
		if !entry.output.Validate(secret, *state) {
			entry.Logger().WithField("file", filename).Error("Write succeeded, but IsValidOnDisk returned false")

			// Remove inconsistent/invalid sync state, consider whatever we've written to be bad.
			// We'll thus rewrite next iteration.
			delete(entry.SyncState, filename)
		} else {
			// TODO: Distinguish added vs changed, using `present` above
			updated.Added++
		}
	}
	// For all secrets we've previously synced, remove state for ones not returned
	for filename := range entry.SyncState {
		if _, present := secrets[filename]; !present {
			pendingDeletions = append(pendingDeletions, filename)
		}
	}
	for _, filename := range pendingDeletions {
		entry.Logger().WithField("secret", filename).Info("Removing old secret")
		delete(entry.SyncState, filename)
		if err := entry.output.Remove(filename); err != nil {
			entry.Logger().WithError(err).Warnf("Unable to delete file")
		} else {
			updated.Deleted++
		}
	}

	deleted, err := entry.output.Cleanup(secrets)
	if err != nil {
		entry.Logger().WithError(err).Warnf("Error cleaning up?")
	}
	updated.Deleted += deleted

	return updated, nil
}
