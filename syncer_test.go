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
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/square/go-sq-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncerLoadClients(t *testing.T) {
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

func TestSyncerBuildClient(t *testing.T) {
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
	entry, err = syncer.buildClient("missingkey", ClientConfig{Mountpoint: "missingkey", Cert: "fixtures/clients/client4.crt"}, &sqmetrics.SquareMetrics{})
	require.NotNil(t, err)

	entry, err = syncer.buildClient("missingcert", ClientConfig{Mountpoint: "missingcert", Key: "fixtures/clients/client4.key"}, &sqmetrics.SquareMetrics{})
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

func TestSyncerRunOnce(t *testing.T) {
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

	// Load a config with the server's URL

	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	// Reset the syncer's URL to point to the mocked server, which has a different port each time
	serverURL, _ := url.Parse(server.URL)
	syncer.server = serverURL
	syncer.config.CaFile = "fixtures/CA/localhost.crt"

	err = syncer.RunOnce()
	require.Nil(t, err)
}

func TestSyncerEntrySync(t *testing.T) {
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

	// Load a config with the server's URL

	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	// Reset the syncer's URL to point to the mocked server, which has a different port each time
	serverURL, _ := url.Parse(server.URL)
	syncer.server = serverURL
	syncer.config.CaFile = "fixtures/CA/localhost.crt"

	err = syncer.LoadClients()
	require.Nil(t, err)

	for name, entry := range syncer.clients {
		err = entry.Sync()
		require.Nil(t, err, "No error expected updating entry %s", name)
	}
}
