// Copyright 2015 Square Inc.
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
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jpillora/backoff"
	pkgerr "github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"github.com/square/go-sq-metrics"
)

// Cipher suites enabled in the client. No RC4 or 3DES.
var ciphers = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
}

// Client represents an interface to a secrets storage backend.
type Client interface {
	Secret(name string) (secret *Secret, err error)
	SecretList() (map[string]Secret, error)
	Logger() *logrus.Entry
	RebuildClient() error
}

// KeywhizHTTPClient is a client that reads from a Keywhiz server over HTTP (v2 API).
type KeywhizHTTPClient struct {
	logger      *logrus.Entry
	httpClient  *http.Client
	url         *url.URL
	params      httpClientParams
	failCount   metrics.Counter
	lastSuccess metrics.Gauge
}

// httpClientParams are values necessary for constructing a TLS client.
type httpClientParams struct {
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	CaBundle   string `json:"ca_bundle"`
	timeout    time.Duration
	maxRetries int
	minBackoff time.Duration
	maxBackoff time.Duration
}

// SecretDeleted is returned as an error when the server 404s.
type SecretDeleted struct{}

func (e SecretDeleted) Error() string {
	return "deleted"
}

func (c KeywhizHTTPClient) failCountInc() {
	c.failCount.Inc(1)
}

func (c KeywhizHTTPClient) markSuccess() {
	c.failCount.Clear()
	c.lastSuccess.Update(time.Now().Unix())
}

// Logger returns the underlying logger for this client
func (c KeywhizHTTPClient) Logger() *logrus.Entry {
	return c.logger
}

// NewClient produces a ready-to-use client struct given client config and
// CA file with the list of trusted certificate authorities.
func NewClient(cfg *ClientConfig, caFile string, serverURL *url.URL, logger *logrus.Entry, metricsHandle *sqmetrics.SquareMetrics) (client Client, err error) {
	logger = logger.WithField("logger", "kwfs_client")

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return &KeywhizHTTPClient{}, fmt.Errorf("bad timeout value '%s': %+v\n", cfg.Timeout, err)
	}

	minBackoff, err := time.ParseDuration(cfg.MinBackoff)
	if err != nil {
		return &KeywhizHTTPClient{}, fmt.Errorf("bad min backoff value '%s': %+v\n", cfg.MinBackoff, err)
	}

	maxBackoff, err := time.ParseDuration(cfg.MaxBackoff)
	if err != nil {
		return &KeywhizHTTPClient{}, fmt.Errorf("bad max backoff value '%s': %+v\n", cfg.MaxBackoff, err)
	}

	params := httpClientParams{
		CertFile:   cfg.Cert,
		KeyFile:    cfg.Key,
		CaBundle:   caFile,
		timeout:    timeout,
		maxRetries: int(cfg.MaxRetries),
		minBackoff: minBackoff,
		maxBackoff: maxBackoff,
	}

	failCount := metrics.GetOrRegisterCounter("runtime.server.fails", metricsHandle.Registry)
	lastSuccess := metrics.GetOrRegisterGauge("runtime.server.lastsuccess", metricsHandle.Registry)

	initial, err := params.buildClient()
	if err != nil {
		return &KeywhizHTTPClient{}, err
	}

	return &KeywhizHTTPClient{logger, initial, serverURL, params, failCount, lastSuccess}, nil
}

// RebuildClient reloads certificates from disk.  It should be called periodically to ensure up-to-date client
// certificates are used.  This is important if you're using short-lived certificates that are routinely replaced.
func (c *KeywhizHTTPClient) RebuildClient() error {
	client, err := c.params.buildClient()
	if err != nil {
		return err
	}
	c.httpClient = client
	return nil
}

// ServerStatus returns raw JSON from the server's _status endpoint
func (c KeywhizHTTPClient) ServerStatus() (data []byte, err error) {
	path := "_status"
	logger := c.logger.WithField("logger", path)
	now := time.Now()
	resp, err := c.getWithRetry(path)
	if err != nil {
		logger.WithError(err).Warn("Error retrieving server status")
		return nil, err
	}
	logger.Infof("GET /%s %d %v", path, resp.StatusCode, time.Since(now))
	logger.WithFields(logrus.Fields{
		"StatusCode": resp.StatusCode,
		"duration":   time.Since(now),
	}).Infof("GET /%s", path)
	defer resp.Body.Close()

	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.WithError(err).Warn("Error reading server status response")
		return nil, err
	}
	return data, nil
}

// RawSecret returns raw JSON from requesting a secret.
func (c KeywhizHTTPClient) RawSecret(name string) ([]byte, error) {
	// note: path.Join does not know how to properly escape for URLs!
	pathname := path.Join("secret", name)
	now := time.Now()
	resp, err := c.getWithRetry(pathname)
	if err != nil {
		c.logger.Errorf("Error retrieving secret %v: %v", name, err)
		c.failCountInc()
		return nil, err
	}
	c.logger.Infof("GET /%s %d %v", pathname, resp.StatusCode, time.Since(now))
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.logger.Errorf("Error reading response body for secret %v: %v", name, err)
		c.failCountInc()
		return nil, err
	}

	switch resp.StatusCode {
	case 200:
		c.markSuccess()
		return data, nil
	case 404:
		c.logger.Warnf("Secret %v not found", name)
		return nil, SecretDeleted{}
	default:
		msg := strings.Join(strings.Split(string(data), "\n"), " ")
		c.logger.Errorf("Bad response code getting secret %v: (status=%v, msg='%s')", name, resp.StatusCode, msg)
		c.failCountInc()
		return nil, errors.New(msg)
	}
}

// Secret returns an unmarshalled Secret struct after requesting a secret.
func (c KeywhizHTTPClient) Secret(name string) (secret *Secret, err error) {
	data, err := c.RawSecret(name)
	if err != nil {
		return nil, err
	}

	secret, err = ParseSecret(data)
	if err != nil {
		return nil, fmt.Errorf("Error decoding retrieved secret %v: %v", name, err)
	}

	return secret, nil
}

// RawSecretList returns raw JSON from requesting a listing of secrets.
func (c KeywhizHTTPClient) RawSecretList() ([]byte, error) {
	path := "secrets"
	now := time.Now()
	resp, err := c.getWithRetry(path)
	if err != nil {
		c.failCountInc()
		return nil, fmt.Errorf("Error retrieving secrets: %v", err)
	}
	c.logger.Infof("GET /%s %d %v", path, resp.StatusCode, time.Since(now))
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.failCountInc()
		return nil, fmt.Errorf("Error reading response body for secrets: %v", err)
	}

	if resp.StatusCode != 200 {
		msg := strings.Join(strings.Split(string(data), "\n"), " ")
		c.failCountInc()
		return nil, fmt.Errorf("Bad response code getting secrets: (status=%v, msg='%s')", resp.StatusCode, msg)
	}
	c.markSuccess()
	return data, nil
}

// SecretList returns a map of unmarshalled Secret structs after requesting a listing of secrets.
// The map keys are the names of the secrets
func (c KeywhizHTTPClient) SecretList() (map[string]Secret, error) {
	data, err := c.RawSecretList()
	if err != nil {
		return nil, err
	}

	secretList, err := ParseSecretList(data)
	if err != nil {
		return nil, fmt.Errorf("Error decoding retrieved secrets: %v", err)
	}
	secrets := map[string]Secret{}
	for _, secret := range secretList {
		filename, err := secret.Filename()
		if err != nil {
			return nil, pkgerr.Wrap(err, "unable to get secret's filename")
		}
		if duplicate, ok := secrets[filename]; ok {
			// This is not supported by Keysync. This stops syncing until the data inconsistency is fixed in the server.
			return nil, fmt.Errorf("Duplicate filename detected: %s on secrets %s and %s",
				filename, duplicate.Name, secret.Name)
		}
		secrets[filename] = secret
	}
	return secrets, nil
}

// buildClient constructs a new TLS client.
func (p httpClientParams) buildClient() (*http.Client, error) {
	keyPair, err := tls.LoadX509KeyPair(p.CertFile, p.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("Error loading Keypair '%s'/'%s': %v", p.CertFile, p.KeyFile, err)
	}

	caCert, err := ioutil.ReadFile(p.CaBundle)
	if err != nil {
		return nil, fmt.Errorf("Error loading CA file '%s': %v", p.CaBundle, err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	config := &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12, // TLSv1.2 and up is required
		CipherSuites: ciphers,
	}
	config.BuildNameToCertificate()
	transport := &http.Transport{TLSClientConfig: config}
	return &http.Client{Transport: transport, Timeout: p.timeout}, nil
}

// shouldRetry decides wether a request should be retried or not.
// e.g. 500 is an intermittent error, but 404 is most likely not.
func shouldRetry(resp *http.Response) bool {
	return resp.StatusCode >= 500
}

// getWithRetry encapsulates the retry logic for requests that failed, because of
// intermittent issues
func (c *KeywhizHTTPClient) getWithRetry(url string) (resp *http.Response, err error) {
	t := *c.url
	t.Path = path.Join(c.url.Path, url)

	b := &backoff.Backoff{
		//These are the defaults
		Min:    c.params.minBackoff,
		Max:    c.params.maxBackoff,
		Jitter: true,
	}

	for i := 0; i < c.params.maxRetries; i++ {
		now := time.Now()
		resp, err = c.httpClient.Get(t.String())
		if err != nil || !shouldRetry(resp) {
			return
		}
		sleep := b.Duration()
		c.logger.Infof("GET /%s %d %v, attempt %d out of %d, retry in %v\n", url, resp.StatusCode, time.Since(now), i+1, c.params.maxRetries, sleep)

		time.Sleep(sleep)
	}

	return
}
