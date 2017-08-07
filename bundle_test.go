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
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupBundleReader(t *testing.T) {
	newAssert := assert.New(t)

	client, err := NewBackupBundleClient("fixtures/exportedSecretsBackupBundle.json", logrus.NewEntry(logrus.New()))
	require.Nil(t, err)

	secret, err := client.Secret("Hacking_Password")
	require.Nil(t, err)
	newAssert.Equal("Hacking_Password", secret.Name)
	newAssert.Equal("0444", secret.Mode)

	secret, err = client.Secret("General_Password")
	require.Nil(t, err)
	newAssert.Equal("General_Password", secret.Name)

	list, err := client.SecretList()
	require.Nil(t, err)
	newAssert.Equal(2, len(list))
}
