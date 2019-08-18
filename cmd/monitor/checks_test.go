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

package main

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/square/keysync"
	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v2"
)

func setupTestEnvironment(t *testing.T) *keysync.Config {
	secretsDir, err := ioutil.TempDir("", "keysync-test")
	assert.Nil(t, err)

	clientsDir, err := ioutil.TempDir("", "keysync-test")
	assert.Nil(t, err)

	certPath := path.Join(clientsDir, "test-client.crt")
	keyPath := path.Join(clientsDir, "test-client.key")

	assert.NoError(t, os.MkdirAll(path.Join(secretsDir, "client"), 0700))
	assert.NoError(t, ioutil.WriteFile(certPath, []byte(strings.TrimSpace(testClientCert)), 0600))
	assert.NoError(t, ioutil.WriteFile(keyPath, []byte(strings.TrimSpace(testClientKey)), 0600))

	config := &keysync.Config{
		SecretsDir: secretsDir,
		ClientsDir: clientsDir,
		YamlExt:    ".yaml",
		Monitor: keysync.MonitorConfig{
			MinCertLifetime: 10 * 365 * 24 * time.Hour,
			MinSecretsCount: 1,
		},
	}

	client := struct {
		Client *keysync.ClientConfig `yaml:"client"`
	}{
		Client: &keysync.ClientConfig{
			Cert: certPath,
			Key:  keyPath,
		},
	}

	clientYAML, err := yaml.Marshal(client)
	assert.Nil(t, err)

	clientPath := path.Join(clientsDir, "client.yaml")
	assert.NoError(t, ioutil.WriteFile(clientPath, clientYAML, 0600))

	return config
}

func cleanupTestEnvironment(t *testing.T, config *keysync.Config) {
	os.RemoveAll(config.SecretsDir)
	os.RemoveAll(config.ClientsDir)
}

func assertError(t *testing.T, errs []error, expected string) {
	for _, err := range errs {
		if strings.Contains(err.Error(), expected) {
			return
		}
	}

	t.Fatalf("expected error '%s', but was not in error list: %q", expected, errs)
}

func TestCheckClientHealth(t *testing.T) {
	config := setupTestEnvironment(t)
	defer cleanupTestEnvironment(t, config)

	errs := checkClientHealth(config)

	// No secrets created by default, check that we caught that.
	assertError(t, errs, "client appears to have zero secrets")

	// Min lifetime is set to be ten years, so the cert should alert.
	assertError(t, errs, "expiring client certificate")
}
