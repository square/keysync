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
)

var testLog = logrus.New().WithField("blah", "blah")

// TestNewOwnership verifies basic functionality, with no fallback or errors
func TestNewOwnership(t *testing.T) {
	var groupFile = "fixtures/ownership/group"
	var passwdFile = "fixtures/ownership/passwd"

	ownership := NewOwnership("test1", "group0", "", "", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 1001, ownership.UID)
	assert.EqualValues(t, 2000, ownership.GID)

	ownership = NewOwnership("test2", "group2", "", "", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 2002, ownership.GID)
}

func TestFallback(t *testing.T) {
	groupFile := "fixtures/ownership/group"
	passwdFile := "fixtures/ownership/passwd"

	ownership := NewOwnership("user-doesnt-exist", "group0", "test1", "group-doesnt-exist", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 1001, ownership.UID)
	assert.EqualValues(t, 2000, ownership.GID)

	ownership = NewOwnership("test2", "group-doesnt-exist", "user-doesnt-exist", "group2", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 2002, ownership.GID)

	ownership = NewOwnership("test2", "group-doesnt-exist", "user-doesnt-exist", "more-nonexist", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 0, ownership.GID)

	ownership = NewOwnership("user-doesnt-exist", "group1", "user-doesnt-exist2", "", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 0, ownership.UID)
	assert.EqualValues(t, 2001, ownership.GID)

	ownership = NewOwnership("", "", "test2", "group2", passwdFile, groupFile, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 2002, ownership.GID)
}

// Verify we return an error if a file is missing
func TestFileMissing(t *testing.T) {
	_, err := lookupUID("group1", "non-existant-file")
	assert.Error(t, err)

	_, err = lookupGID("group1", "non-existant-file")
	assert.Error(t, err)
}

// Verify we return an error for users and groups not present
func TestLookupFailure(t *testing.T) {
	groupFile := "fixtures/ownership/group"
	passwdFile := "fixtures/ownership/passwd"
	_, err := lookupUID("non-existent", passwdFile)
	assert.Error(t, err)

	_, err = lookupUID("non-existent", groupFile)
	assert.Error(t, err)
}

// Verify bad rows return an error.  Good rows are tested in all above tests
func TestCorruptData(t *testing.T) {
	groupFile := "fixtures/ownership/group"
	passwdFile := "fixtures/ownership/passwd"
	_, err := lookupGID("badgroup", groupFile)
	assert.Error(t, err)

	_, err = lookupUID("baddata", passwdFile)
	assert.Error(t, err)
}
