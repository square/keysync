// Copyright 2015 Square Inc.
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
	"github.com/sirupsen/logrus"
)

// Ownership indicates the default ownership of filesystem entries.
type Ownership struct {
	UID int
	GID int
	// Where to look up users and groups
	Lookup
}

// NewOwnership initializes default file ownership struct.
// Logs as error anything that goes wrong, but always returns something
// Worst-case you get "0", ie root, owning things, which is safe as root can always read all files.
func NewOwnership(username, groupname, fallbackUser, fallbackGroup string, lookup Lookup, logger *logrus.Entry) Ownership {
	var uid, gid int
	var err error

	if username != "" {
		uid, err = lookup.UID(username)
	}
	if err != nil {
		logger.WithError(err).WithField("user", username).Error("Error looking up username, using fallback")
	}
	if username == "" || err != nil {
		uid, err = lookup.UID(fallbackUser)
		if err != nil {
			uid = 0
			logger.WithError(err).WithField("user", fallbackUser).Error("Error looking up fallback username, using 0")
		}
	}

	if groupname != "" {
		gid, err = lookup.GID(groupname)
	}
	if err != nil {
		logger.WithError(err).WithField("group", groupname).Error("Error looking up groupname, using fallback")
	}
	if groupname == "" || err != nil {
		gid, err = lookup.GID(fallbackGroup)
		if err != nil {
			gid = 0
			logger.WithError(err).WithField("group", fallbackGroup).Error("Error looking up fallback groupname, using 0")
		}
	}

	return Ownership{
		UID:    uid,
		GID:    gid,
		Lookup: lookup,
	}
}
