// Copyright 2016 Square Inc.
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

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// atomicWrite creates a temporary file, sets perms, writes content, and renames it to filename
// This sequence ensures the following:
// 1. Nobody can open the file before we set owner/permissions properly
// 2. Nobody observes a partially-overwritten secret file.
// Since keysync is intended to write to tmpfs, this function doesn't do the necessary fsyncs if it
// were persisting content to disk.
func atomicWrite(name string, secret *Secret, defaultOwner Ownership) error {
	// We can't use ioutil.TempFile because we want to open 0000.
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return err
	}
	rand := hex.EncodeToString(buf)
	f, err := os.OpenFile(name+rand, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0000)
	if err != nil {
		return err
	}
	ownership := secret.OwnershipValue(defaultOwner)
	err = f.Chown(int(ownership.Uid), int(ownership.Gid))
	if err != nil {
		// TODO: We will fail as non-root/CAP_CHOWN. Bad in prod, but don't want to test as root.
		fmt.Printf("Chown failed: %v\n", err)
	}
	// Always Chmod after the Chown, so we don't expose secret with the wrong owner.
	err = f.Chmod(os.FileMode(secret.ModeValue()))
	if err != nil {
		return err
	}

	_, err = f.Write(secret.Content)
	if err != nil {
		return err
	}

	// Rename is atomic, so nobody will observe a partially updated secret
	err = os.Rename(name+rand, name)
	return err
}
