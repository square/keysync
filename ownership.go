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
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

var groupFile = "/etc/group"

// Ownership indicates the default ownership of filesystem entries.
type Ownership struct {
	UID uint32
	GID uint32
}

// NewOwnership initializes default file ownership struct.
func NewOwnership(username, groupname string) (Ownership, error) {
	uid, err := lookupUID(username)
	if err != nil {
		return Ownership{}, err
	}
	gid, err := lookupGID(groupname)
	if err != nil {
		return Ownership{}, err
	}
	return Ownership{
		UID: uid,
		GID: gid,
	}, nil
}

// lookupUID resolves a username to a numeric id.
func lookupUID(username string) (uint32, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, fmt.Errorf("Error resolving uid for %v: %v\n", username, err)
	}

	uid, err := strconv.ParseUint(u.Uid, 10 /* base */, 32 /* bits */)
	if err != nil {
		return 0, fmt.Errorf("Error resolving uid for %v: %v\n", username, err)
	}

	return uint32(uid), nil
}

// lookupGID resolves a groupname to a numeric id.
func lookupGID(groupname string) (uint32, error) {
	gid, err := lookupGidInFile(groupname, groupFile)
	if err != nil {
		return 0, fmt.Errorf("Error resolving gid for %s: %v\n", groupname, err)
	}

	return gid, nil
}

func lookupGidInFile(groupname, fileName string) (uint32, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return 0, fmt.Errorf("Error opening groupFile %s: %v\n", groupFile, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := strings.Split(scanner.Text(), ":")
		if entry[0] == groupname && len(entry) >= 3 {
			gid, err := strconv.ParseUint(entry[2], 10 /* base */, 32 /* bits */)
			if err != nil {
				return 0, err
			}
			return uint32(gid), nil
		}
	}

	return 0, errors.New("no such group")
}
