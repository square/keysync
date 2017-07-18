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

package keysync

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
)

// Ownership indicates the default ownership of filesystem entries.
type Ownership struct {
	UID uint32
	GID uint32
	// Where to look up users and groups
	userSource  string
	groupSource string
}

// NewOwnership initializes default file ownership struct.
// Logs as error anything that goes wrong, but always returns something
// Worst-case you get "0", ie root, owning things, which is safe as root can always read all files.
func NewOwnership(username, groupname, fallbackUser, fallbackGroup, passwdFile, groupFile string, logger *logrus.Entry) Ownership {
	var uid, gid uint32
	var err error

	if username != "" {
		uid, err = lookupUID(username, passwdFile)
	}
	if err != nil {
		logger.WithError(err).WithField("user", username).Error("Error looking up username, using fallback")
	}
	if username == "" || err != nil {
		uid, err = lookupUID(fallbackUser, passwdFile)
		if err != nil {
			uid = 0
			logger.WithError(err).WithField("user", fallbackUser).Error("Error looking up fallback username, using 0")
		}
	}

	if groupname != "" {
		gid, err = lookupGID(groupname, groupFile)
	}
	if err != nil {
		logger.WithError(err).WithField("group", groupname).Error("Error looking up groupname, using fallback")
	}
	if groupname == "" || err != nil {
		gid, err = lookupGID(fallbackGroup, groupFile)
		if err != nil {
			gid = 0
			logger.WithError(err).WithField("group", fallbackGroup).Error("Error looking up fallback groupname, using 0")
		}
	}

	return Ownership{
		UID:         uid,
		GID:         gid,
		userSource:  passwdFile,
		groupSource: groupFile,
	}
}

// lookupUID resolves a username to a numeric id.
func lookupUID(username, passwdFile string) (uint32, error) {
	uid, err := lookupIDInFile(username, passwdFile)
	if err != nil {
		return 0, fmt.Errorf("Error resolving uid for %s: %v\n", username, err)
	}

	return uid, nil
}

// lookupGID resolves a groupname to a numeric id.
func lookupGID(groupname, groupFile string) (uint32, error) {
	gid, err := lookupIDInFile(groupname, groupFile)
	if err != nil {
		return 0, fmt.Errorf("Error resolving gid for %s: %v\n", groupname, err)
	}

	return gid, nil
}

func lookupIDInFile(name, fileName string) (uint32, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return 0, fmt.Errorf("Error opening file %s: %v\n", fileName, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := strings.Split(scanner.Text(), ":")
		if entry[0] == name && len(entry) >= 3 {
			gid, err := strconv.ParseUint(entry[2], 10 /* base */, 32 /* bits */)
			if err != nil {
				return 0, err
			}
			return uint32(gid), nil
		}
	}

	return 0, fmt.Errorf("%s not found in %s", name, fileName)
}
