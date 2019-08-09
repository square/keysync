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

package ownership

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var testLog = logrus.New().WithField("test", "test")

var data = Mock{
	Users:  map[string]uint32{"test0": 1000, "test1": 1001, "test2": 1002},
	Groups: map[string]uint32{"group0": 2000, "group1": 2001, "group2": 2002},
}

// TestNewOwnership verifies basic functionality, with no fallback or errors
func TestNewOwnership(t *testing.T) {
	ownership := NewOwnership("test1", "group0", "", "", &data, testLog)
	assert.EqualValues(t, 1001, ownership.UID)
	assert.EqualValues(t, 2000, ownership.GID)

	ownership = NewOwnership("test2", "group2", "", "", &data, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 2002, ownership.GID)
}

func TestFallback(t *testing.T) {
	ownership := NewOwnership("user-doesnt-exist", "group0", "test1", "group-doesnt-exist", &data, testLog)
	assert.EqualValues(t, 1001, ownership.UID)
	assert.EqualValues(t, 2000, ownership.GID)

	ownership = NewOwnership("test2", "group-doesnt-exist", "user-doesnt-exist", "group2", &data, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 2002, ownership.GID)

	ownership = NewOwnership("test2", "group-doesnt-exist", "user-doesnt-exist", "more-nonexist", &data, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 0, ownership.GID)

	ownership = NewOwnership("user-doesnt-exist", "group1", "user-doesnt-exist2", "", &data, testLog)
	assert.EqualValues(t, 0, ownership.UID)
	assert.EqualValues(t, 2001, ownership.GID)

	ownership = NewOwnership("", "", "test2", "group2", &data, testLog)
	assert.EqualValues(t, 1002, ownership.UID)
	assert.EqualValues(t, 2002, ownership.GID)
}

// Verify we return an error for users and groups not present
func TestLookupFailure(t *testing.T) {
	lookup := Os{}
	_, err := lookup.UID("non-existent")
	assert.Error(t, err)

	_, err = lookup.UID("non-existent")
	assert.Error(t, err)

	_, err = lookup.UID("#test2")
	assert.Error(t, err)
}
