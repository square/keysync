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

	os.MkdirAll(path.Join(secretsDir, "client"), 0700)
	ioutil.WriteFile(certPath, []byte(strings.TrimSpace(testClientCert)), 0600)
	ioutil.WriteFile(keyPath, []byte(strings.TrimSpace(testClientKey)), 0600)

	config := &keysync.Config{
		SecretsDir:      secretsDir,
		ClientsDir:      clientsDir,
		YamlExt:         ".yaml",
		MinCertLifetime: 10 * 365 * 24 * time.Hour,
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
	ioutil.WriteFile(clientPath, clientYAML, 0600)

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

	t.Fatalf("expected error '%s', but was not in error list", expected)
}

func TestCheckClientHealth(t *testing.T) {
	config := setupTestEnvironment(t)
	defer cleanupTestEnvironment(t, config)

	errs := checkClientHealth(config)

	// No secrets created by default, check that we caught that.
	assertError(t, errs, "client appears to have zero secrets")

	// Min lifetime is set to be ten years, so the cert should alert.
	assertError(t, errs, "expired/expiring key/cert in config for client")
}
