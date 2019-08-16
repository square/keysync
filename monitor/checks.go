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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"syscall"
	"time"

	"github.com/square/keysync"
)

func checkPaths(config *keysync.Config) []error {
	errs := []error{}
	errs = append(errs, directoryExists(config.SecretsDir)...)
	errs = append(errs, directoryExists(config.ClientsDir)...)
	errs = append(errs, fileExists(config.CaFile)...)
	return errs
}

func checkClientHealth(config *keysync.Config) []error {
	clients, err := config.LoadClients()
	if err != nil {
		return []error{fmt.Errorf("unable to load clients: %s", err)}
	}

	errs := []error{}
	for name, client := range clients {
		// MinCertLifetime, if not set in the config, will default to zero.
		// In that case this check will still work but only alert if the
		// certificate is *already* expired.
		if err := checkCertificate(name, &client, config.Monitor.MinCertLifetime); err != nil {
			errs = append(errs, err)
		}

		// Check that each client has at least one secret. It makes no
		// sense to have a client without secrets, so if there's an empty
		// client dir something is probably wrong.
		if err := checkHasSecrets(name, &client, config.SecretsDir, config.Monitor.MinSecretsCount); err != nil {
			errs = append(errs, err)
		}

	}

	return errs
}

func checkCertificate(name string, client *keysync.ClientConfig, minCertLifetime time.Duration) error {
	if client.Key == "" {
		return fmt.Errorf("no key specified in config for client %s", name)
	}

	keyPair, err := tls.LoadX509KeyPair(client.Cert, client.Key)
	if err != nil {
		return fmt.Errorf("unable to load certificate and key for client %s: %s", name, err)
	}

	leaf, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return fmt.Errorf("invalid client certificate for client %s: %s", name, err)
	}

	if leaf.NotAfter.Before(time.Now()) {
		return fmt.Errorf("expired client certificate for client %s: NotAfter %s", name, leaf.NotAfter.Format(time.RFC3339))
	}

	if expiryThreshold := time.Now().Add(minCertLifetime); leaf.NotAfter.Before(expiryThreshold) {
		return fmt.Errorf("expiring client certificate for client %s: NotAfter %s is within %s of now", name, leaf.NotAfter.Format(time.RFC3339), minCertLifetime)
	}

	return nil
}

func checkHasSecrets(name string, client *keysync.ClientConfig, secretsDir string, minSecretsCount int) error {
	dir := path.Join(secretsDir, client.DirName)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("unable to open secrets dir for client %s: %s", name, err)
	}

	if len(files) < minSecretsCount {
		return fmt.Errorf("client %s appears to have zero secrets", name)
	}

	return nil
}

func checkServerHealth(config *keysync.Config) []error {
	url := fmt.Sprintf("http://localhost:%d/status", config.APIPort)

	resp, err := http.Get(url)
	if err != nil {
		return []error{fmt.Errorf("unable to talk to status: %s", err)}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []error{fmt.Errorf("unable to talk to status: %s", err)}
	}

	status := &keysync.StatusResponse{}
	err = json.Unmarshal(body, &status)
	if err != nil {
		return []error{fmt.Errorf("invalid JSON status response: %s", err)}
	}

	if !status.Ok {
		return []error{fmt.Errorf("keysync unhealthy: %s", status.Message)}
	}

	return nil
}

func checkDiskUsage(config *keysync.Config) []error {
	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(config.SecretsDir, &fs); err != nil {
		return []error{fmt.Errorf("could not statfs secrets dir: %v", err)}
	}

	// Relative free space is number of free blocks divided by number of total blocks
	freeSpace := float64(fs.Bfree) / float64(fs.Blocks)
	if freeSpace < 0.1 {
		return []error{fmt.Errorf("disk usage of '%s' is above 90%% (blocks: %d free, %d total)", config.SecretsDir, fs.Bfree, fs.Blocks)}
	}

	return nil
}

func fileExists(path string) []error {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return []error{fmt.Errorf("expected '%s' to be a file", path)}
	}
	return nil
}

func directoryExists(path string) []error {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return []error{fmt.Errorf("expected '%s' to be a directory", path)}
	}
	return nil
}
