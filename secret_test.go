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
	"time"

	"os"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSecretDeserializeSecret(t *testing.T) {
	newAssert := assert.New(t)

	s, err := ParseSecret(fixture("secret.json"))
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
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	defaultOwnership := Ownership{UID: 1, GID: 1}

	ownership := Secret{Owner: "test0"}.OwnershipValue(defaultOwnership)
	newAssert.EqualValues(ownership.UID, 1234)
	newAssert.EqualValues(ownership.GID, 1)

	ownership = Secret{Owner: "test1", Group: "test2"}.OwnershipValue(defaultOwnership)
	newAssert.EqualValues(ownership.UID, 1235)
	newAssert.EqualValues(ownership.GID, 1236)

	ownership = Secret{}.OwnershipValue(defaultOwnership)
	newAssert.EqualValues(ownership.UID, 1)
	newAssert.EqualValues(ownership.GID, 1)
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
