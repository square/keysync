// Copyright 2017 Square Inc.
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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncerLoadClients(t *testing.T) {
	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, NewInMemoryOutputCollection(), logrus.NewEntry(logrus.New()), metricsForTest())
	require.Nil(t, err)

	_, err = syncer.LoadClients()
	require.Nil(t, err)

	// The clients should reload without error
	_, err = syncer.LoadClients()
	require.Nil(t, err)
}

func TestSyncerLoadClientsError(t *testing.T) {
	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, NewInMemoryOutputCollection(), logrus.NewEntry(logrus.New()), metricsForTest())
	require.Nil(t, err)

	_, err = syncer.LoadClients()
	require.NotNil(t, err)
}

func TestSyncerBuildClient(t *testing.T) {
	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, NewInMemoryOutputCollection(), logrus.NewEntry(logrus.New()), metricsForTest())
	require.Nil(t, err)

	clients, err := config.LoadClients()
	require.Nil(t, err)

	client1, ok := clients["client1"]
	require.True(t, ok)

	entry, err := syncer.buildClient("client1", client1, metricsForTest())
	require.Nil(t, err)
	assert.Equal(t, entry.ClientConfig, client1)

	// Test misconfigured clients
	cfg := defaultClientConfig()
	cfg.DirName = "missingkey"
	cfg.Cert = "fixtures/clients/client4.crt"
	cfg.Key = ""
	entry, err = syncer.buildClient("missingkey", *cfg, metricsForTest())
	require.Error(t, err)
	require.Nil(t, entry)

	cfg = defaultClientConfig()
	cfg.DirName = "missingcert"
	cfg.Cert = ""
	cfg.Key = "fixtures/clients/client4.key"
	entry, err = syncer.buildClient("missingcert", *cfg, metricsForTest())
	require.Error(t, err)
	require.Nil(t, entry)

	// The syncer currently handles clients configured with missing mountpoints
	cfg = defaultClientConfig()
	cfg.DirName = "valid"
	cfg.Cert = "fixtures/clients/client4.crt"
	cfg.Key = "fixtures/clients/client4.key"
	entry, err = syncer.buildClient("missingcert", *cfg, metricsForTest())
	require.NoError(t, err)
	require.NotNil(t, entry)
}

func TestSyncerRandomDuration(t *testing.T) {
	testData := []struct{ start, end string }{
		{"100s", "125s"},
		{"10s", "12.5s"},
		{"1s", "1.25s"},
		{"21h", "26.25h"},
	}
	for j := 1; j <= 1024; j++ {
		for _, interval := range testData {
			start, err := time.ParseDuration(interval.start)
			if err != nil {
				t.Fatalf("Parsing test data: %v", err)
			}
			end, err := time.ParseDuration(interval.end)
			if err != nil {
				t.Fatalf("Parsing test data: %v", err)
			}
			random := randomize(start)
			if float64(random) < float64(start) {
				t.Fatalf("Random before expected range: %v < %v", random, start)
			}
			if float64(random) > float64(end) {
				t.Fatalf("Random beyond expected range: %v > %v", random, end)
			}
		}
	}
}

func TestSyncerRunSuccess(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	updated, errs := syncer.RunOnce()
	require.Nil(t, errs)

	// For each client, we should have added two secrets.
	require.Equal(t, len(syncer.clients)*2, int(updated.Added), "Expect two files added per client")

	for _, entry := range syncer.clients {
		// Check the files in the mountpoint
		output := entry.output.(*InMemoryOutput)
		require.Equal(t, 2, len(output.Secrets), "Expect two files successfully written after sync")

		_, present := output.Secrets["Nobody_PgPass"]
		assert.True(t, present, "Expect Nobody_PgPass successfully written after sync")

		_, present = output.Secrets["General_Password..0be68f903f8b7d86"]
		assert.True(t, present, "Expect General_Password..0be68f903f8b7d86 successfully written after sync")
	}
}

func TestSyncChangedSecrets(t *testing.T) {
	type secret struct {
		Name   string `json:"name"`
		Secret string `json:"secret"`
	}

	// Create a new server that generates a random value for a secret named changing-secret every time
	// a hander is called. This lets us simulate secret changes.
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bytes := make([]byte, 8)
		rand.Read(bytes)
		s := &secret{
			Name:   "changing-secret",
			Secret: hex.EncodeToString(bytes),
		}

		encoder := json.NewEncoder(w)
		switch r.URL.Path {
		case "/secrets":
			s.Secret = ""
			encoder.Encode([]*secret{s})
		case "/secret/changing-secret":
			encoder.Encode(s)
		case "/batchsecret":
			encoder.Encode([]*secret{s})
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	// The first time, all secrets should be added.
	updated, errs := syncer.RunOnce()
	require.Nil(t, errs)
	require.Equal(t, Updated{Added: uint(len(syncer.clients)), Changed: 0, Deleted: 0}, updated)

	// The next time, all secrets should changed.
	updated, errs = syncer.RunOnce()
	require.Nil(t, errs)
	require.Equal(t, Updated{Added: 0, Changed: uint(len(syncer.clients)), Deleted: 0}, updated)
}

func TestSyncerRunSuccessWithDeletionRace(t *testing.T) {
	server := createDefaultServerWithDeletionRace()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	// Clear the syncer's poll interval so the "Run" loop only executes once
	syncer.pollInterval = 0

	err = syncer.Run()
	require.Nil(t, err)

	// Only one secret should have been written because the other was deleted
	for _, entry := range syncer.clients {
		// Check the files in the mountpoint
		output := entry.output.(*InMemoryOutput)
		require.Equal(t, 1, len(output.Secrets), "Expect one file successfully written after sync")

		_, present := output.Secrets["Nobody_PgPass"]
		assert.True(t, present, "Expect Nobody_PgPass successfully written after sync")
	}
}

func TestSyncerRunLoadClientsFails(t *testing.T) {
	server := createDefaultServerWithDeletionRace()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml", server)
	require.Nil(t, err)

	// Clear the syncer's poll interval so the "Run" loop only executes once
	syncer.pollInterval = 0

	err = syncer.Run()
	require.NotNil(t, err)
}

func TestNewSyncerFails(t *testing.T) {
	// Load a test config which fails on LoadClients
	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	// Set an invalid server URL
	config.Server = "\\"

	_, err = NewSyncer(config, OutputDirCollection{}, logrus.NewEntry(logrus.New()), metricsForTest())
	require.NotNil(t, err)
}

// Simulates a Keywhiz server outage leading to 500 errors.  The secrets should not be deleted
// from the mountpoint for Keywhiz-internal errors, but should be deleted when the response is 404.
func TestSyncerEntrySyncKeywhizFails(t *testing.T) {
	server := createDefaultServerWithDeletionRace()
	defer server.Close()

	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	_, err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		_, err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint
		output := entry.output.(*InMemoryOutput)
		require.Equal(t, 1, len(output.Secrets), "Expect one file successfully written after sync")
		_, present := output.Secrets["Nobody_PgPass"]
		assert.True(t, present, "Expect Nobody_PgPass successfully written after sync")
	}

	// Switch to a server which errors internally when accessing the secret; this should not cause it to be deleted
	internalErrorServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, string(fixture("secrets.json")))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/Nobody_PgPass"):
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	internalErrorServer.TLS = testCerts(testCaFile)
	internalErrorServer.StartTLS()
	defer internalErrorServer.Close()

	resetSyncerServer(syncer, internalErrorServer)

	// Clear and reload the clients to force them to pick up the new server
	syncer.clients = make(map[string]syncerEntry)
	_, err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		_, err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint
		output := entry.output.(*InMemoryOutput)
		require.Equal(t, 1, len(output.Secrets), "Expect one file successfully written after sync")
		_, present := output.Secrets["Nobody_PgPass"]
		assert.True(t, present, "Expect Nobody_PgPass successfully written after sync despite internal error")
	}

	// Switch to a server in which the secret is deleted
	deletedServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, string(fixture("secrets.json")))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/Nobody_PgPass"):
			w.WriteHeader(404)
		default:
			w.WriteHeader(404)
		}
	}))
	deletedServer.TLS = testCerts(testCaFile)
	deletedServer.StartTLS()
	defer deletedServer.Close()

	resetSyncerServer(syncer, deletedServer)

	// Clear and reload the clients to force them to pick up the new server
	syncer.clients = make(map[string]syncerEntry)
	_, err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		_, err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint
		output := entry.output.(*InMemoryOutput)
		require.Equal(t, 0, len(output.Secrets), "Expect all secrets to be deleted after sync")
	}

	// Switch to a server in which the secret has an override that is a filepath
	compromisedServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, string(fixture("secretsWithBadFilenameOverride.json")))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/Nobody_PgPass"):
			w.WriteHeader(404)
		default:
			w.WriteHeader(404)
		}
	}))
	compromisedServer.TLS = testCerts(testCaFile)
	compromisedServer.StartTLS()
	defer compromisedServer.Close()

	resetSyncerServer(syncer, compromisedServer)

	// Clear and reload the clients to force them to pick up the new server
	syncer.clients = make(map[string]syncerEntry)
	syncer.outputCollection = NewInMemoryOutputCollection()
	_, err = syncer.LoadClients()
	require.Nil(t, err)

	for _, entry := range syncer.clients {
		_, err = entry.Sync()
		require.NotNil(t, err)

		// Check the files in the mountpoint
		output := entry.output.(*InMemoryOutput)
		require.Equal(t, 0, output.NumWrites(), "Expect no secrets to be written during sync")
		require.Equal(t, 0, output.NumDeletes(), "Expect no secrets to be deleted after sync")
	}
}
