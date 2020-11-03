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
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/square/keysync/ownership"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSecretDeserializeSecret(t *testing.T) {
	newAssert := assert.New(t)

	s, err := ParseSecret(fixture("secret_Nobody_PgPass.json"))
	require.Nil(t, err)
	newAssert.Equal("Nobody_PgPass", s.Name)
	newAssert.EqualValues(6, s.Length)
	newAssert.Equal("0400", s.Mode)
	newAssert.Equal("nobody", s.Owner)
	newAssert.Equal("nobody", s.Group)
	newAssert.EqualValues("asddas", s.Content)

	expectedCreatedAt := time.Date(2011, time.September, 29, 15, 46, 0, 232000000, time.UTC)
	newAssert.Equal(s.CreatedAt.Unix(), expectedCreatedAt.Unix())
}

func TestSecretDeserializeSecretWithoutBase64Padding(t *testing.T) {
	newAssert := assert.New(t)

	s, err := ParseSecret(fixture("secretWithoutBase64Padding.json"))
	require.Nil(t, err)
	newAssert.Equal("NonexistentOwner_Pass", s.Name)
	newAssert.EqualValues("12345", s.Content)
}

func TestSecretDeserializeSecretList(t *testing.T) {
	newAssert := assert.New(t)

	fixtures := []string{"secrets.json", "secretsWithoutContent.json"}
	for _, f := range fixtures {
		secrets, err := ParseSecretList(fixture(f))
		require.Nil(t, err)
		newAssert.Len(secrets, 2)
	}
}

func TestSecretModeValue(t *testing.T) {
	newAssert := assert.New(t)

	cases := []struct {
		secret Secret
		mode   uint32
	}{
		{Secret{Mode: "0440"}, 288},
		{Secret{Mode: "0400"}, 256},
		{Secret{}, 288},
	}
	for _, c := range cases {
		mode, err := c.secret.ModeValue()
		require.Nil(t, err)
		newAssert.Equal(os.FileMode(c.mode|unix.S_IFREG), mode)
	}

	_, err := Secret{Mode: "9999"}.ModeValue()
	newAssert.NotNil(err)
}

func TestSecretOwnershipValue(t *testing.T) {
	var data = ownership.Mock{
		Users:  map[string]int{"test0": 1000, "test1": 1001, "test2": 1002},
		Groups: map[string]int{"group0": 2000, "group1": 2001, "group2": 2002},
	}
	defaultOwnership := ownership.Ownership{UID: 1, GID: 1, Lookup: &data}

	own := Secret{Owner: "test0"}.OwnershipValue(defaultOwnership)
	assert.EqualValues(t, 1000, own.UID)
	assert.EqualValues(t, 1, own.GID)

	own = Secret{Owner: "test1", Group: "group2"}.OwnershipValue(defaultOwnership)
	assert.EqualValues(t, 1001, own.UID)
	assert.EqualValues(t, 2002, own.GID)

	own = Secret{}.OwnershipValue(defaultOwnership)
	assert.EqualValues(t, 1, own.UID)
	assert.EqualValues(t, 1, own.GID)
}

func TestContentErrors(t *testing.T) {
	s, err := ParseSecret(fixture("secretWithoutBase64Padding.json"))
	require.Nil(t, err)
	originalContent := make([]byte, len(s.Content))
	copy(originalContent, s.Content) // Save the original content of this secret

	data, err := json.Marshal(12)
	require.Nil(t, err)
	err = s.Content.UnmarshalJSON(data)
	assert.NotNil(t, err)
	assert.EqualValues(t, originalContent, s.Content)

	raw := json.RawMessage(`"not base64"`)
	data, err = json.Marshal(&raw)
	require.Nil(t, err)
	err = s.Content.UnmarshalJSON(data)
	assert.NotNil(t, err)
	assert.EqualValues(t, originalContent, s.Content)
}

func TestFilename(t *testing.T) {
	filenameOverride := "../../deleteme"
	s := Secret{Name: "mydbsecret", FilenameOverride: &filenameOverride}
	name, err := s.Filename()
	require.NotNil(t, err)
	require.Empty(t, name)

	s = Secret{Name: "../../mydbsecret"}
	name, err = s.Filename()
	require.NotNil(t, err)
	require.Empty(t, name)

	s = Secret{Name: "mydbsecret"}
	name, err = s.Filename()
	require.Nil(t, err)
	require.Equal(t, "mydbsecret", name)

	filenameOverride = "fileoverride"
	s = Secret{Name: "mydbsecret", FilenameOverride: &filenameOverride}
	name, err = s.Filename()
	require.Nil(t, err)
	require.Equal(t, "fileoverride", name)
}
