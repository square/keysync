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

	"github.com/sirupsen/logrus"
	"github.com/square/go-sq-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	clientCert = "fixtures/clients/client1.crt"
	clientKey  = "fixtures/clients/client1.key"
	testCaFile = "fixtures/CA/localhost.crt"
)

func defaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Key:        clientKey,
		Cert:       clientCert,
		MaxRetries: 1,
		Timeout:    "1s",
		MinBackoff: "1ms",
		MaxBackoff: "10ms",
	}
}

func TestClientCallsServer(t *testing.T) {
	newAssert := assert.New(t)

	server := createDefaultServer()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	secrets, err := client.SecretList()
	newAssert.Nil(err)
	newAssert.Len(secrets, 2)

	data, err := client.(*KeywhizHTTPClient).RawSecretList()
	newAssert.Nil(err)
	newAssert.Equal(fixture("secrets.json"), data)

	secret, err := client.Secret("Nobody_PgPass")
	require.Nil(t, err)
	newAssert.Equal("Nobody_PgPass", secret.Name)

	data, err = client.(*KeywhizHTTPClient).RawSecret("Nobody_PgPass")
	require.Nil(t, err)
	newAssert.Equal(fixture("secret.json"), data)

	_, err = client.Secret("unexisting")
	_, deleted := err.(SecretDeleted)
	newAssert.True(deleted)
}

func TestClientRebuild(t *testing.T) {
	serverURL, _ := url.Parse("http://dummy:8080")
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	http1 := client.(*KeywhizHTTPClient).httpClient
	err = client.RebuildClient()
	require.Nil(t, err)
	http2 := client.(*KeywhizHTTPClient).httpClient

	if http1 == http2 {
		t.Error("should not be same http client")
	}
}

func TestClientCallsServerErrors(t *testing.T) {
	newAssert := assert.New(t)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			w.WriteHeader(500)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/500-error"):
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	secrets, err := client.SecretList()
	newAssert.NotNil(err)
	newAssert.Len(secrets, 0)

	data, err := client.(*KeywhizHTTPClient).RawSecretList()
	assert.Error(t, err)
	assert.Nil(t, data)

	secret, err := client.Secret("bar")
	newAssert.Nil(secret)
	_, deleted := err.(SecretDeleted)
	newAssert.True(deleted)

	data, err = client.(*KeywhizHTTPClient).RawSecret("bar")
	newAssert.Nil(data)
	newAssert.Error(err)
	_, deleted = err.(SecretDeleted)
	newAssert.True(deleted)

	data, err = client.(*KeywhizHTTPClient).RawSecret("500-error")
	newAssert.Nil(data)
	newAssert.True(err != nil)
	_, deleted = err.(SecretDeleted)
	newAssert.False(deleted)

	_, err = client.Secret("non-existent")
	newAssert.Nil(data)
	_, deleted = err.(SecretDeleted)
	newAssert.True(deleted)
}

func TestClientCallsServerIntermittentErrors(t *testing.T) {
	newAssert := assert.New(t)

	numCalls := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			// simulate intermittent error
			if numCalls > 0 {
				fmt.Fprint(w, `[{"name" : "SecretA", "filename": "filenameA"},
			 {"name" : "SecretB", "filename": "filenameB"}]`)
				w.WriteHeader(200)
			} else {
				w.WriteHeader(500)
			}
		default:
			w.WriteHeader(404)
		}
		numCalls++
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	cfg := defaultClientConfig()
	cfg.MaxRetries = 2
	client, err := NewClient(cfg, testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	secrets, err := client.SecretList()
	newAssert.Nil(err)
	newAssert.Len(secrets, 2)
}

// Test a server that returns invalid secret JSON information
func TestClientCorruptedResponses(t *testing.T) {
	newAssert := assert.New(t)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, "hi")
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secret/foo"):
			fmt.Fprint(w, "hi again")
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	_, err = client.SecretList()
	newAssert.NotNil(err)

	_, err = client.Secret("foo")
	require.NotNil(t, err)
}

func TestClientParsingError(t *testing.T) {
	newAssert := assert.New(t)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	secrets, err := client.SecretList()
	newAssert.NotNil(err)
	newAssert.Len(secrets, 0)
}

func TestClientServerStatusSuccess(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/_status"):
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	_, err = client.(*KeywhizHTTPClient).ServerStatus()
	require.Nil(t, err)
}

func TestClientServerFailure(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	_, err = client.(*KeywhizHTTPClient).ServerStatus()
	require.NotNil(t, err)

	_, err = client.Secret("secret")
	require.NotNil(t, err)

	_, err = client.SecretList()
	require.NotNil(t, err)
}

func TestNewClientFailure(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-ca-file-config.yaml")
	require.Nil(t, err)

	clientConfigs, err := config.LoadClients()
	require.Nil(t, err)

	// Try to load a client with an invalid CA file configured
	clientName := "client1"
	serverURL, _ := url.Parse(server.URL)
	cfg := defaultClientConfig()
	cfg.Cert = clientConfigs[clientName].Cert
	cfg.Key = clientConfigs[clientName].Key
	_, err = NewClient(cfg, config.CaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	assert.NotNil(t, err)
}

func TestDuplicateFilenames(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/secrets"):
			fmt.Fprint(w, `[{"name" : "SecretA", "filename": "overridden_filename"},
			 {"name" : "SecretB", "filename": "overridden_filename"}]`)
		default:
			w.WriteHeader(404)
		}
	}))
	server.TLS = testCerts(testCaFile)
	server.StartTLS()
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	client, err := NewClient(defaultClientConfig(), testCaFile, serverURL, logrus.NewEntry(logrus.New()), &sqmetrics.SquareMetrics{})
	require.Nil(t, err)

	_, err = client.SecretList()
	assert.EqualError(t, err, "duplicate filename detected: overridden_filename on secrets SecretA and SecretB")
}
