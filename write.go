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

package keysync

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// WriteConfig stores the options for atomicWrite
type WriteConfig struct {
	WriteDirectory    string
	DefaultOwnership  Ownership
	EnforceFilesystem Filesystem // What filesystem type do we expect to write to?
	ChownFiles        bool       // Do we chown the file? (Needs root or CAP_CHOWN).
}

// FileInfo returns the filesystem properties atomicWrite wrote
type FileInfo struct {
	os.FileMode
	UID uint32
	GID uint32
}

// GetFileInfo from an open file
func GetFileInfo(file *os.File) (*FileInfo, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("Failed to stat after writing: %v", err)
	}
	filemode := stat.Mode()
	uid := stat.Sys().(*syscall.Stat_t).Uid
	gid := stat.Sys().(*syscall.Stat_t).Gid

	return &FileInfo{filemode, uid, gid}, nil
}

// atomicWrite creates a temporary file, sets perms, writes content, and renames it to filename
// This sequence ensures the following:
// 1. Nobody can open the file before we set owner/permissions properly
// 2. Nobody observes a partially-overwritten secret file.
// Since keysync is intended to write to tmpfs, this function doesn't do the necessary fsyncs if it
// were persisting content to disk.
func atomicWrite(name string, secret *Secret, writeConfig WriteConfig) (*FileInfo, error) {
	if strings.ContainsRune(name, filepath.Separator) {
		// This prevents a secret named "../../etc/passwd" from being written outside this directory
		return nil, fmt.Errorf("Cannot write: %s contains %c", name, filepath.Separator)
	}
	// We can't use ioutil.TempFile because we want to open 0000.
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}
	randSuffix := hex.EncodeToString(buf)
	fullPath := filepath.Join(writeConfig.WriteDirectory, name)
	f, err := os.OpenFile(fullPath+randSuffix, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0000)
	// Try to remove the file, in event we early-return with an error.
	defer os.Remove(fullPath + randSuffix)
	if err != nil {
		return nil, err
	}

	if writeConfig.ChownFiles {
		ownership := secret.OwnershipValue(writeConfig.DefaultOwnership)

		err = f.Chown(int(ownership.UID), int(ownership.GID))
		if err != nil {
			return nil, err
		}
	}

	mode, err := secret.ModeValue()
	if err != nil {
		return nil, err
	}

	// Always Chmod after the Chown, so we don't expose secret with the wrong owner.
	err = f.Chmod(mode)
	if err != nil {
		return nil, err

	}

	if writeConfig.EnforceFilesystem != 0 {
		good, err := isFilesystem(f, writeConfig.EnforceFilesystem)
		if err != nil {
			return nil, fmt.Errorf("Checking filesystem type: %v", err)
		}
		if !good {
			return nil, fmt.Errorf("Unexpected filesystem writing %s", name)
		}
	}
	_, err = f.Write(secret.Content)
	if err != nil {
		return nil, fmt.Errorf("Failed writing filesystem content: %v", err)
	}

	filemode, err := GetFileInfo(f)

	// While this is intended for use with tmpfs, you could write secrets to disk.
	// We ignore any errors from syncing, as it's not strictly required.
	_ = f.Sync()

	// Rename is atomic, so nobody will observe a partially updated secret
	err = os.Rename(fullPath+randSuffix, fullPath)
	if err != nil {
		return nil, err
	}
	return filemode, nil
}

// The Filesystem identification.  On Mac, this is uint32, and int64 on linux
// So both are safe to store as an int64.
// Linux Tmpfs = 0x01021994
// Get these constants with `stat --file-system --format=%t`
type Filesystem int64

func isFilesystem(file *os.File, fs Filesystem) (bool, error) {
	var statfs syscall.Statfs_t
	err := syscall.Fstatfs(int(file.Fd()), &statfs)
	return Filesystem(statfs.Type) == fs, err
}
