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
	"net/http"
	"testing"

	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/square/go-sq-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiSyncAllAndSyncClientSuccess(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	port := uint16(4444) // Shutting down the APIServer at the end of the test would require changing the method to return a pointer to the server

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, string(fixture("secrets.json")))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/Nobody_PgPass"):
			fmt.Fprint(w, string(fixture("secret.json")))
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	// Load a test config
	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	// Reset the syncer's URL to point to the mocked server, which has a different port each time
	serverURL, _ := url.Parse(server.URL)
	syncer.server = serverURL
	syncer.config.CaFile = "fixtures/CA/localhost.crt"

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()))

	// Test SyncAll success
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/sync", port), nil)
	require.Nil(t, err)

	_, err = http.DefaultClient.Do(req)
	require.Nil(t, err)

	// TODO: Check returned data

	// Test SyncClientsuccess
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/sync/client1", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	// TODO: Check returned data

	// Test SyncClient failure on nonexistent client
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/sync/non-existent", port), nil)
	require.Nil(t, err)

	res, err = http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	// TODO: Check returned data
}

func TestApiSyncOneError(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	port := uint16(4446)

	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	err = syncer.LoadClients()
	assert.NotNil(t, err)

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()))

	// Test error loading clients when syncing single client
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/sync/client1", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

	// Test error loading clients when syncing all clients
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/sync", port), nil)
	require.Nil(t, err)

	res, err = http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
}

func TestHealthCheck(t *testing.T) {
	// TODO: Make this function more complex as the health check improves
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	port := uint16(4444) // This will reuse the "success" server when run with that test

	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	err = syncer.LoadClients()
	assert.NotNil(t, err)

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()))

	// Check health under good conditions
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/_status", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}
