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
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/rcrowley/go-metrics"
	"github.com/square/go-sq-metrics"
)

// Create metrics for testing purposes
func metricsForTest() *sqmetrics.SquareMetrics {
	return sqmetrics.NewMetrics("", "test", nil, 1*time.Second, metrics.DefaultRegistry, &log.Logger{})
}

// Create a new server that returns "secrets.json" and "secret.json" for its endpoints
// Users should call defer server.close immediately after getting this server.
func createDefaultServer() *httptest.Server {
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
	return server
}

// Create a new syncer with the given config and server, failing for any
func createNewSyncer(configFile string, server *httptest.Server) (*Syncer, error) {
	// Load a config with the server's URL
	config, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	syncer, err := NewSyncer(config, logrus.NewEntry(logrus.New()), metricsForTest())
	if err != nil {
		return nil, err
	}
	syncer.config.CaFile = "fixtures/CA/localhost.crt"

	return resetSyncerServer(syncer, server), nil
}

// Reset the given syncer's server URL to point to the given server
func resetSyncerServer(syncer *Syncer, server *httptest.Server) *Syncer {
	serverURL, _ := url.Parse(server.URL)
	syncer.server = serverURL
	return syncer
}

// fixture fully reads test data from a file in the fixtures/ subdirectory.
func fixture(file string) (content []byte) {
	content, err := ioutil.ReadFile("fixtures/" + file)
	panicOnError(err)
	return
}

// Load the file with cert & private key into a tls.Config
func testCerts(file string) (config *tls.Config) {
	config = new(tls.Config)
	cert, err := tls.LoadX509KeyPair(file, file)
	panicOnError(err)

	config.Certificates = []tls.Certificate{cert}

	return config
}

// Helper function to panic on error
func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
