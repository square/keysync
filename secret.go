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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/square/keysync/ownership"

	"os"

	"golang.org/x/sys/unix"
)

// ParseSecret deserializes raw JSON into a Secret struct.
func ParseSecret(data []byte) (s *Secret, err error) {
	if err = json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("Fail to deserialize JSON Secret: %v", err)
	}
	return
}

// ParseSecretList deserializes raw JSON into a list of Secret structs.
func ParseSecretList(data []byte) (secrets []Secret, err error) {
	if err = json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("Fail to deserialize JSON []Secret: %v", err)
	}
	return
}

// Secret represents data returned after processing a server request.
//
// json tags after fields indicate to json decoder the key name in JSON
type Secret struct {
	Name             string
	Content          content   `json:"secret"`
	Length           uint64    `json:"secretLength"`
	Checksum         string    `json:"checksum"`
	CreatedAt        time.Time `json:"creationDate"`
	UpdatedAt        time.Time `json:"updateDate"`
	FilenameOverride *string   `json:"filename"`
	Mode             string
	Owner            string
	Group            string
}

// ModeValue function helps by converting a textual mode to the expected value for fuse.
func (s Secret) ModeValue() (os.FileMode, error) {
	mode := s.Mode
	if mode == "" {
		mode = "0440"
	}
	modeValue, err := strconv.ParseUint(mode, 8 /* base */, 16 /* bits */)
	if err != nil {
		return 0, fmt.Errorf("Unable to parse secret file mode (%v): %v\n", mode, err)
	}
	// The only acceptable bits to set in a mode are read bits, so we mask off any additional bits.
	modeValue = modeValue & 0444
	return os.FileMode(modeValue | unix.S_IFREG), nil
}

// OwnershipValue returns the ownership for a given secret, falling back to the values given as
// an argument if they're not present in the secret
func (s Secret) OwnershipValue(fallback ownership.Ownership) (ret ownership.Ownership) {
	ret = fallback
	if s.Owner != "" {
		uid, err := fallback.Lookup.UID(s.Owner)
		if err == nil {
			ret.UID = uid
		}
	}
	if s.Group != "" {
		gid, err := fallback.Lookup.GID(s.Group)
		if err == nil {
			ret.GID = gid
		}
	}
	return
}

// Filename returns the expected filename of a secret. The filename metadata overrides the name,
// but it can't be path, so keysync can't delete or write arbitrary files outside its secrets directory.
func (s Secret) Filename() (string, error) {
	name := s.Name
	if s.FilenameOverride != nil {
		name = *s.FilenameOverride
	}

	if strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("secret has invalid filename, got '%s'", name)
	}

	return name, nil
}

// content is a helper type used to convert base64-encoded data from the server.
type content []byte

func (c *content) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("secret should be a string, got '%s' (%v)", data, err)
	}

	// Go's base64 requires padding to be present so we add it if necessary.
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("secret not valid base64, got '%+v' (%v)", s, err)
	}

	*c = decoded
	return nil
}
