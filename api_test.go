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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiSyncAllAndSyncClientSuccess(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	port := uint16(4444) // Shutting down the APIServer at the end of the test would require changing the method to return a pointer to the server

	server := createDefaultServer()
	defer server.Close()

	// Load a test config
	syncer, err := createNewSyncer("fixtures/configs/test-config.yaml", server)
	require.Nil(t, err)

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()), metricsForTest())

	// Test SyncAll success
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/sync", port), nil)
	require.Nil(t, err)

	_, err = http.DefaultClient.Do(req)
	require.Nil(t, err)

	// TODO: Check returned data

	// Test SyncClientsuccess
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/sync/client1", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	// TODO: Check returned data

	// Test SyncClient failure on nonexistent client
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/sync/non-existent", port), nil)
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

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), metricsForTest())
	require.Nil(t, err)

	err = syncer.LoadClients()
	assert.NotNil(t, err)

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()), metricsForTest())

	// Test error loading clients when syncing single client
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/sync/client1", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

	// Test error loading clients when syncing all clients
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/sync", port), nil)
	require.Nil(t, err)

	res, err = http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
}

func TestHealthCheck(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	port := uint16(4445)

	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), metricsForTest())
	require.Nil(t, err)

	err = syncer.LoadClients()
	assert.NotNil(t, err)

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()), metricsForTest())

	// 1. Check that health check returns false if we've never had a success
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/status", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)

	// 2. Check health is true under good conditions (make it look like there was a successful sync)
	syncer.updateSuccessTimestamp()

	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/status", port), nil)
	require.Nil(t, err)

	res, err = http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestMetricsReporting(t *testing.T) {
	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	port := uint16(4444) // This will reuse the "success" server when run with that test

	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), metricsForTest())
	require.Nil(t, err)

	err = syncer.LoadClients()
	assert.NotNil(t, err)

	NewAPIServer(syncer, port, logrus.NewEntry(logrus.New()), metricsForTest())

	// Check health under good conditions
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/metrics", port), nil)
	require.Nil(t, err)

	res, err := http.DefaultClient.Do(req)
	require.Nil(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// Check that metrics is valid JSON (should be an array)
	body, _ := ioutil.ReadAll(res.Body)
	var parsed []interface{}
	err = json.Unmarshal(body, &parsed)

	if err != nil {
		t.Errorf("output from /metrics is not valid JSON, though it should be: %s", err)
	}
}
