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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/sirupsen/logrus"
	sqmetrics "github.com/square/go-sq-metrics"
)

// Create metrics for testing purposes
func metricsForTest() *sqmetrics.SquareMetrics {
	return sqmetrics.NewMetrics("", "test", nil, 1*time.Second, metrics.DefaultRegistry, &log.Logger{})
}

// Create a new server that returns "secretsWithoutContents.json", "secret.json", and "secrets.json" for its endpoints
// Users should call defer server.close immediately after getting this server.
func createDefaultServer() *httptest.Server {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, string(fixture("secretsWithoutContent.json")))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/Nobody_PgPass"):
			fmt.Fprint(w, string(fixture("secret.json")))
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/batchsecret"):
			if requestContainsExpectedSecrets(r) {
				fmt.Fprint(w, string(fixture("secrets.json")))
			} else {
				// The "secrets.json" file is only a valid response if the two secrets in it were requested
				w.WriteHeader(400)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	return server
}

func requestContainsExpectedSecrets(r *http.Request) bool {
	body, err := ioutil.ReadAll(r.Body)
	panicOnError(err)
	var req = map[string][]string{}
	err = json.Unmarshal(body, &req)
	panicOnError(err)
	secrets, ok := req["secrets"]
	return ok && contains(secrets, "Nobody_PgPass") && contains(secrets, "General_Password..0be68f903f8b7d86")
}

func contains(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

// Create a new syncer with the given config and server, failing for any
func createNewSyncer(configFile string, server *httptest.Server) (*Syncer, error) {
	// Load a config with the server's URL
	config, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	syncer, err := NewSyncer(config, NewInMemoryOutputCollection(), logrus.NewEntry(logrus.New()), metricsForTest())
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

// Pass this to syncer to get an "in memory output", which records how secrets are written, making this useful
// for testing behaviour without ever writing secrets to disk anywhere.
type InMemoryOutputCollection struct {
	Outputs map[string]InMemoryOutput
}

var _ OutputCollection = InMemoryOutputCollection{}

func NewInMemoryOutputCollection() InMemoryOutputCollection {
	return InMemoryOutputCollection{Outputs: map[string]InMemoryOutput{}}
}

func (c InMemoryOutputCollection) NewOutput(clientConfig ClientConfig, logger *logrus.Entry) (Output, error) {
	name := clientConfig.DirName
	if previous, present := c.Outputs[name]; present {
		return previous, nil
	}
	output := InMemoryOutput{Secrets: map[string]Secret{}, logger: logger}

	logger.Warn("Making new client for ", name)
	c.Outputs[name] = output
	logger.Warnf("clients: %v", c.Outputs)
	return output, nil
}

func (c InMemoryOutputCollection) Cleanup(_ map[string]struct{}, _ *logrus.Entry) (uint, []error) {
	return 0, nil
}

type InMemoryOutput struct {
	logger         *logrus.Entry
	Secrets        map[string]Secret
	writesCounter  int
	deletesCounter int
}

func (out InMemoryOutput) Validate(secret *Secret, state secretState) bool {
	_, present := out.Secrets[secret.Name]
	// If it's in the map, it's valid - delete from the map to test on-disk invalidation behavior.
	return present
}

func (out InMemoryOutput) Write(secret *Secret) (*secretState, error) {
	out.Secrets[secret.Name] = *secret
	out.writesCounter++
	out.logger.WithField("muhname", secret.Name).Warn("writing secret")
	return &secretState{}, nil
}

func (out InMemoryOutput) Remove(name string) error {
	delete(out.Secrets, name)
	out.deletesCounter++
	out.logger.WithField("mahnuum", name).Warn("deleting secret")
	return nil
}

func (out InMemoryOutput) RemoveAll() (uint, error) {
	deleted := uint(len(out.Secrets))
	out.deletesCounter += len(out.Secrets)
	out.Secrets = map[string]Secret{}
	return deleted, nil
}

func (out InMemoryOutput) Cleanup(_ map[string]Secret) (uint, error) {
	return 0, nil
}

func (out InMemoryOutput) Logger() *logrus.Entry {
	return nil
}

func (out InMemoryOutput) NumWrites() int {
	return out.writesCounter
}

func (out InMemoryOutput) NumDeletes() int {
	return out.deletesCounter
}
