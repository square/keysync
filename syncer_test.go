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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/square/go-sq-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncerLoadClients(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	err = syncer.LoadClients()
	require.Nil(t, err)

	// The clients should reload without error
	err = syncer.LoadClients()
	require.Nil(t, err)
}

func TestSyncerLoadClientsError(t *testing.T) {
	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	err = syncer.LoadClients()
	require.NotNil(t, err)
}

func TestSyncerBuildClient(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	clients, err := config.LoadClients()
	require.Nil(t, err)

	client1, ok := clients["client1"]
	require.True(t, ok)

	entry, err := syncer.buildClient("client1", client1, &sqmetrics.SquareMetrics{})
	require.Nil(t, err)
	assert.Equal(t, entry.ClientConfig, client1)

	// Test misconfigured clients
	entry, err = syncer.buildClient("missingkey", ClientConfig{DirName: "missingkey", Cert: "fixtures/clients/client4.crt"}, &sqmetrics.SquareMetrics{})
	require.NotNil(t, err)

	entry, err = syncer.buildClient("missingcert", ClientConfig{DirName: "missingcert", Key: "fixtures/clients/client4.key"}, &sqmetrics.SquareMetrics{})
	require.NotNil(t, err)

	// The syncer currently handles clients configured with missing mountpoints
	entry, err = syncer.buildClient("missingcert", ClientConfig{Key: "fixtures/clients/client4.key", Cert: "fixtures/clients/client4.crt"}, &sqmetrics.SquareMetrics{})
	require.Nil(t, err)
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
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	server := createDefaultServer()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	// Clear the syncer's poll interval so the "Run" loop only executes once
	syncer.config.PollInterval = ""

	err = syncer.Run()
	require.Nil(t, err)
}

func TestSyncerRunLoadClientsFails(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml", server)
	require.Nil(t, err)

	// Clear the syncer's poll interval so the "Run" loop only executes once
	syncer.config.PollInterval = ""

	err = syncer.Run()
	require.NotNil(t, err)
}

func TestNewSyncerFails(t *testing.T) {
	// Load a test config which fails on LoadCLients
	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	// Set an invalid server URL
	config.Server = "\\"

	_, err = NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.NotNil(t, err)
}

func TestSyncerRunOnce(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	server := createDefaultServer()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	err = syncer.RunOnce()
	require.Nil(t, err)
}

func TestSyncerEntrySync(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	// Create a new syncer with this server
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint (based on fixtures/secrets.json)
		fileInfos, err := ioutil.ReadDir(entry.WriteDirectory)
		require.Nil(t, err, "No error expected reading directory %s", entry.WriteDirectory)
		require.Equal(t, 1, len(fileInfos), "Expect one file successfully written after sync", entry.WriteDirectory)
		assert.Equal(t, "Nobody_PgPass", fileInfos[0].Name(), "Expect one file successfully written after sync")
	}
}

func TestSyncerDirectory(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	require.Nil(t, syncer.LoadClients())
	require.Nil(t, syncer.RunOnce())

	// Verify we write to the correct directories
	for _, file := range []string{"fixtures/secrets/client1/Nobody_PgPass", "fixtures/secrets/client4_overridden/Nobody_PgPass"} {
		b, err := ioutil.ReadFile(file)
		require.Nil(t, err)
		require.Equal(t, b, []byte("asddas"))
	}
}

func TestSyncerEntrySyncWrite(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	err = syncer.LoadClients()
	require.Nil(t, err)

	// This should log a warning when trying to write secrets, but should not return an error
	for name, entry := range syncer.clients {
		// Set the entry to chown files; should log an error but not fail when testing
		entry.WriteConfig.ChownFiles = true
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check that no files were written to the mountpoint
		fileInfos, err := ioutil.ReadDir(entry.WriteDirectory)
		require.Nil(t, err, "No error expected reading directory %s", entry.WriteDirectory)
		require.Equal(t, 0, len(fileInfos), "Expect no files successfully written after sync")
	}
}

func TestSyncerEntrySyncWriteFail(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	err = syncer.LoadClients()
	require.Nil(t, err)

	// This should log a warning when trying to write secrets, but should not return an error
	for name, entry := range syncer.clients {
		// Set the entry to check its location; should log an error but not fail when testing
		entry.WriteConfig.EnforceFilesystem = 0x01
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check that no files were written to the mountpoint
		fileInfos, err := ioutil.ReadDir(entry.WriteDirectory)
		require.Nil(t, err, "No error expected reading directory %s", entry.WriteDirectory)
		require.Equal(t, 0, len(fileInfos), "Expect no files successfully written after sync")
	}
}

// Simulates a Keywhiz server outage leading to 500 errors.  The secrets should not be deleted
// from the mountpoint for Keywhiz-internal errors, but should be deleted when the response is 404.
func TestSyncerEntrySyncKeywhizFails(t *testing.T) {
	server := createDefaultServer()
	defer server.Close()

	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint
		fileInfos, err := ioutil.ReadDir(entry.WriteDirectory)
		require.Nil(t, err, "No error expected reading directory %s", entry.WriteDirectory)
		require.Equal(t, 1, len(fileInfos), "Expect one file successfully written after sync")
		assert.Equal(t, "Nobody_PgPass", fileInfos[0].Name(), "Expect Nobody_PgPass successfully written after sync")
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
	err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint
		fileInfos, err := ioutil.ReadDir(entry.WriteDirectory)
		require.Nil(t, err, "No error expected reading directory %s", entry.DirName)
		require.Equal(t, 1, len(fileInfos), "Expect one file still successfully written after sync")
		assert.Equal(t, "Nobody_PgPass", fileInfos[0].Name(), "Expect Nobody_PgPass successfully written after sync despite internal error")
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
	err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)

		// Check the files in the mountpoint
		fileInfos, err := ioutil.ReadDir(entry.WriteDirectory)
		require.Nil(t, err, "No error expected reading directory %s", entry.WriteDirectory)
		require.Equal(t, 0, len(fileInfos), "Expect all secrets to be deleted after sync")
	}
}
