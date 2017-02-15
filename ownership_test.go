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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnershipNewOwnership(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	ownership, err := NewOwnership("test1", "test0")
	require.Nil(t, err)
	newAssert.EqualValues(1234, ownership.GID)
	newAssert.EqualValues(1235, ownership.UID)

	ownership, err = NewOwnership("test2", "test2")
	require.Nil(t, err)
	newAssert.EqualValues(1236, ownership.GID)
	newAssert.EqualValues(1236, ownership.UID)
}

func TestOwnershipGroupFileParsingValid(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	gid, err := lookupGID("test0")
	newAssert.Nil(err)
	newAssert.EqualValues(1234, gid)

	gid, err = lookupGID("test1")
	newAssert.Nil(err)
	newAssert.EqualValues(1235, gid)

	gid, err = lookupGID("test2")
	newAssert.Nil(err)
	newAssert.EqualValues(1236, gid)
}

func TestOwnershipOwnerFileParsingValid(t *testing.T) {
	newAssert := assert.New(t)

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	gid, err := lookupUID("test0")
	newAssert.Nil(err)
	newAssert.EqualValues(1234, gid)

	gid, err = lookupUID("test1")
	newAssert.Nil(err)
	newAssert.EqualValues(1235, gid)

	gid, err = lookupUID("test2")
	newAssert.Nil(err)
	newAssert.EqualValues(1236, gid)
}

func TestOwnershipGroupFileMissing(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "non-existent"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	_, err := lookupGID("test1")
	newAssert.NotNil(err)

	_, err = NewOwnership("test1", "test0")
	newAssert.NotNil(err)
}

func TestOwnershipPasswdFileMissing(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "non-existent"
	defer func() { passwdFile = "/etc/passwd" }()

	_, err := lookupUID("test1")
	newAssert.NotNil(err)

	_, err = NewOwnership("test2", "test2")
	newAssert.NotNil(err)
}

func TestOwnershipGroupNotPresent(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	_, err := lookupGID("non-existent")
	newAssert.NotNil(err)

	_, err = NewOwnership("test0", "non-existent")
	newAssert.NotNil(err)
}

func TestOwnershipUserNotPresent(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	_, err := lookupUID("non-existent")
	newAssert.NotNil(err)

	_, err = NewOwnership("non-existent", "test0")
	newAssert.NotNil(err)
}

func TestOwnershipGroupCorrupted(t *testing.T) {
	newAssert := assert.New(t)

	groupFile = "fixtures/ownership/group"
	defer func() { groupFile = "/etc/group" }()

	_, err := lookupGID("test3")
	newAssert.NotNil(err)

	_, err = NewOwnership("test0", "test3")
	newAssert.NotNil(err)
}

func TestOwnershipUserCorrupted(t *testing.T) {
	newAssert := assert.New(t)

	passwdFile = "fixtures/ownership/passwd"
	defer func() { passwdFile = "/etc/passwd" }()

	_, err := lookupUID("test3")
	newAssert.NotNil(err)

	_, err = NewOwnership("test3", "test0")
	newAssert.NotNil(err)
}
