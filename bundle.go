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
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BackupBundleClient is a secrets client that reads from a Keywhiz backup bundle.
type BackupBundleClient struct {
	secrets map[string]Secret
	logger  *logrus.Entry
}

// NewBackupBundleClient creates a new BackupBundleClient instance given a backup JSON file.
func NewBackupBundleClient(path string, logger *logrus.Entry) (Client, error) {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	parsed, err := ParseSecretList(raw)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to parse secret list from path: %s", path))
	}

	client := BackupBundleClient{
		secrets: map[string]Secret{},
		logger:  logger.WithField("logger", "file_client"),
	}

	for _, secret := range parsed {
		name, err := secret.Filename()
		if err != nil {
			return nil, errors.Wrap(err, "unable to get secret's filename")
		}
		client.secrets[name] = secret
	}

	return &client, nil
}

// Secret returns secret with the given name from the bundle.
func (c BackupBundleClient) Secret(name string) (secret *Secret, err error) {
	s, ok := c.secrets[name]
	if !ok {
		return nil, fmt.Errorf("unable to find %s in backup bundle", name)
	}
	return &s, nil
}

// SecretList returns all secrets in a bundle (unlike the real Keywhiz interface,
// it will return secrets' contents as well).
func (c BackupBundleClient) SecretList() (map[string]Secret, error) {
	return c.secrets, nil
}

// SecretListWithContents returns the requested secrets from a bundle.
func (c BackupBundleClient) SecretListWithContents(secrets []string) (map[string]Secret, error) {
	result := map[string]Secret{}
	for _, name := range secrets {
		s, ok := c.secrets[name]
		if !ok {
			return nil, fmt.Errorf("unable to find %s in backup bundle", name)
		}
		result[name] = s
	}
	return result, nil
}

// Logger returns the logger for this client.
func (c BackupBundleClient) Logger() *logrus.Entry {
	return c.logger
}

// RebuildClient for bundle clients is a no-op.
func (c BackupBundleClient) RebuildClient() error {
	return nil
}
