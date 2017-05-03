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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoadConfigSuccess(t *testing.T) {
	newAssert := assert.New(t)

	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	require.Nil(t, err)
	newAssert.Equal("fixtures/clients", config.ClientsDir)
	newAssert.Equal("fixtures/CA/cacert.crt", config.CaFile)
	newAssert.Equal("yaml", config.YamlExt)
	newAssert.False(config.ChownFiles)
	newAssert.Equal("localhost:4444", config.Server)
	newAssert.True(config.Debug)
	newAssert.Equal("keysync-test", config.DefaultUser)
	newAssert.Equal("keysync-test", config.DefaultGroup)
	newAssert.EqualValues(31738, config.APIPort)
	newAssert.Equal("60s", config.PollInterval)

	// TODO: Test loading defaults
}

func TestConfigLoadConfigMissingOrInvalidFiles(t *testing.T) {
	newAssert := assert.New(t)

	_, err := LoadConfig("non-existent")
	newAssert.NotNil(err)

	_, err = LoadConfig("fixtures/configs/errorconfigs/notyaml-test-config.yaml")
	newAssert.NotNil(err)

	// TODO: Uncomment if we add validation for the client dir and CA file
	//_, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	//newAssert.Nil(err)

	//_, err = LoadConfig("fixtures/configs/errorconfigs/nonexistent-ca-file-config.yaml")
	//newAssert.NotNil(err)
}

func TestConfigLoadClientsSuccess(t *testing.T) {
	newAssert := assert.New(t)

	config, err := LoadConfig("fixtures/configs/test-config.yaml")
	newAssert.Nil(err)

	clients, err := config.LoadClients()
	newAssert.Nil(err)

	for _, name := range []string{"client1", "client2", "client3"} {
		client, ok := clients[name]
		newAssert.True(ok)
		newAssert.Equal(name, client.DirName)
		newAssert.Equal(fmt.Sprintf("fixtures/clients/%s.key", name), client.Key)
		newAssert.Equal(fmt.Sprintf("fixtures/clients/%s.crt", name), client.Cert)
	}

	assert.Equal(t, "client4_overridden", clients["client4"].DirName)

	client, ok := clients["missingcert"]
	newAssert.True(ok)
	newAssert.Equal("fixtures/clients/client4.key", client.Key)
	// With no cert specified, it's assumed to be in the key file
	newAssert.Equal("fixtures/clients/client4.key", client.Cert)

	client, ok = clients["owners"]
	newAssert.True(ok)
	newAssert.Equal("fixtures/clients/client1.key", client.Key)
	newAssert.Equal("fixtures/clients/client1.crt", client.Cert)
	newAssert.Equal("test-user", client.User)
	newAssert.Equal("test-group", client.Group)
}

func TestConfigLoadClientsInvalidFiles(t *testing.T) {
	config, err := LoadConfig("fixtures/configs/errorconfigs/nonexistent-client-dir-config.yaml")
	require.Nil(t, err)

	_, err = config.LoadClients()
	assert.NotNil(t, err)

	_, err = LoadConfig("fixtures/configs/errorconfigs/missing-secrets-dir-config.yaml")
	assert.NotNil(t, err)

	config, err = LoadConfig("fixtures/configs/errorconfigs/missingkey-config.yaml")
	require.Nil(t, err)

	_, err = config.LoadClients()
	assert.NotNil(t, err)

	config, err = LoadConfig("fixtures/configs/errorconfigs/notyaml-client-config.yaml")
	require.Nil(t, err)

	_, err = config.LoadClients()
	assert.NotNil(t, err)
}
