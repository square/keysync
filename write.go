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
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/square/keysync/ownership"

	pkgerr "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// OutputCollection handles a collection of outputs.
type OutputCollection interface {
	NewOutput(clientConfig ClientConfig, logger *logrus.Entry) (Output, error)
	// Cleanup unknown clients (eg, ones deleted while keysync was not running)
	Cleanup(map[string]struct{}, *logrus.Entry) []error
}

// Output is an interface that encapsulates what it means to store secrets
type Output interface {
	// Validate returns true if the secret is persisted already
	Validate(secret *Secret, state secretState) bool
	// Write a secret
	Write(secret *Secret) (*secretState, error)
	// Remove a secret
	Remove(name string) error
	// Remove all secrets and the containing directory (eg, when the client config is removed)
	RemoveAll() error
	// Cleanup unknown files (eg, ones deleted in Keywhiz while keysync was not running)
	Cleanup(map[string]Secret) error
}

type OutputDirCollection struct {
	Config *Config
}

func (c OutputDirCollection) NewOutput(clientConfig ClientConfig, logger *logrus.Entry) (Output, error) {
	defaultOwnership := ownership.NewOwnership(
		clientConfig.User,
		clientConfig.Group,
		c.Config.DefaultUser,
		c.Config.DefaultGroup,
		ownership.Os{},
		logger,
	)

	writeDirectory := filepath.Join(c.Config.SecretsDir, clientConfig.DirName)
	if err := os.MkdirAll(writeDirectory, 0775); err != nil {
		return nil, fmt.Errorf("failed to mkdir client directory '%s': %v", writeDirectory, err)
	}

	return &OutputDir{
		WriteDirectory:    writeDirectory,
		EnforceFilesystem: c.Config.FsType,
		ChownFiles:        c.Config.ChownFiles,
		DefaultOwnership:  defaultOwnership,
		Logger:            logger,
	}, nil
}

func (c OutputDirCollection) Cleanup(known map[string]struct{}, logger *logrus.Entry) []error {
	var errors []error

	fileInfos, err := ioutil.ReadDir(c.Config.SecretsDir)
	if err != nil {
		errors = append(errors, err)
		logger.WithError(err).WithField("SecretsDir", c.Config.SecretsDir).Warn("Couldn't read secrets dir")
	}
	for _, fileInfo := range fileInfos {
		if !fileInfo.IsDir() {
			logger.WithField("name", fileInfo.Name()).Warn("Found unknown file, ignoring")
			continue
		}
		if _, present := known[fileInfo.Name()]; !present {
			logger.WithField("name", fileInfo.Name()).WithField("known", known).Warn("Deleting unknown directory")
			os.RemoveAll(filepath.Join(c.Config.SecretsDir, fileInfo.Name()))
		}
	}

	return errors
}

// OutputDir implements Output to files, which is the typical keysync usage to a tmpfs.
type OutputDir struct {
	WriteDirectory    string
	DefaultOwnership  ownership.Ownership
	EnforceFilesystem Filesystem // What filesystem type do we expect to write to?
	ChownFiles        bool       // Do we chown the file? (Needs root or CAP_CHOWN).
	Logger            *logrus.Entry
}

// Validate verifies the secret is written to disk with the correct content, permissions, and ownership
func (out *OutputDir) Validate(secret *Secret, state secretState) bool {
	if state.Checksum != secret.Checksum {
		return false
	}

	filename, err := secret.Filename()
	if err != nil {
		return false
	}
	path := filepath.Join(out.WriteDirectory, filename)

	// Check if new permissions match state
	if state.Owner != secret.Owner || state.Group != secret.Group || state.Mode != secret.Mode {
		return false
	}

	// Check on-disk permissions, and ownership against what's configured.
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	fileinfo, err := GetFileInfo(f)
	if err != nil {
		return false
	}
	if state.FileInfo != *fileinfo {
		out.Logger.WithFields(logrus.Fields{
			"secret":   filename,
			"expected": state.FileInfo,
			"seen":     *fileinfo,
		}).Warn("Secret permissions changed unexpectedly")
		return false
	}

	// Check the content of what's on disk
	var b bytes.Buffer
	_, err = b.ReadFrom(f)
	if err != nil {
		return false
	}
	hash := sha256.Sum256(b.Bytes())

	if state.ContentHash != hash {
		// As tempting as it is, we shouldn't log hashes as they'd leak information about the secret.
		out.Logger.WithField("secret", filename).Warn("Secret modified on disk")
		return false
	}

	// OK, the file is unchanged
	return true

}

func (out *OutputDir) Remove(name string) error {
	return os.Remove(filepath.Join(out.WriteDirectory, name))
}

func (out *OutputDir) RemoveAll() error {
	return os.RemoveAll(out.WriteDirectory)
}

func (out *OutputDir) Cleanup(secrets map[string]Secret) error {
	fileInfos, err := ioutil.ReadDir(out.WriteDirectory)
	if err != nil {
		return fmt.Errorf("couldn't read directory: %s", out.WriteDirectory)
	}
	for _, fileInfo := range fileInfos {
		existingFile := fileInfo.Name()
		if _, present := secrets[existingFile]; !present {
			// This file wasn't written in the loop above, so we remove it.
			out.Logger.WithField("file", existingFile).Info("Removing unknown file")
			err := os.Remove(filepath.Join(out.WriteDirectory, existingFile))
			if err != nil {
				out.Logger.WithError(err).Warnf("Unable to delete file")
			}
		}
	}
	return nil
}

// FileInfo returns the filesystem properties atomicWrite wrote
type FileInfo struct {
	Mode os.FileMode
	UID  uint32
	GID  uint32
}

// GetFileInfo from an open file
func GetFileInfo(file *os.File) (*FileInfo, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat after writing: %v", err)
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
func (out *OutputDir) Write(secret *Secret) (*secretState, error) {
	filename, err := secret.Filename()
	if err != nil {
		return nil, pkgerr.Wrap(err, "cannot write to file")
	}

	if err := os.MkdirAll(out.WriteDirectory, 0775); err != nil {
		return nil, fmt.Errorf("making client directory '%s': %v", out.WriteDirectory, err)
	}

	// We can't use ioutil.TempFile because we want to open 0000.
	buf := make([]byte, 32)
	_, err = rand.Read(buf)
	if err != nil {
		return nil, err
	}
	randSuffix := hex.EncodeToString(buf)
	fullPath := filepath.Join(out.WriteDirectory, filename)
	f, err := os.OpenFile(fullPath+randSuffix, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0000)
	// Try to remove the file, in event we early-return with an error.
	defer os.Remove(fullPath + randSuffix)
	if err != nil {
		return nil, err
	}

	if out.ChownFiles {
		ownership := secret.OwnershipValue(out.DefaultOwnership)

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

	if out.EnforceFilesystem != 0 {
		good, err := isFilesystem(f, out.EnforceFilesystem)
		if err != nil {
			return nil, fmt.Errorf("checking filesystem type: %v", err)
		}
		if !good {
			return nil, fmt.Errorf("unexpected filesystem writing %s", filename)
		}
	}
	_, err = f.Write(secret.Content)
	if err != nil {
		return nil, fmt.Errorf("failed writing filesystem content: %v", err)
	}

	filemode, err := GetFileInfo(f)
	if err != nil {
		return nil, fmt.Errorf("failed to get file mode back from file: %v", err)
	}

	// While this is intended for use with tmpfs, you could write secrets to disk.
	// We ignore any errors from syncing, as it's not strictly required.
	_ = f.Sync()

	// Rename is atomic, so nobody will observe a partially updated secret
	err = os.Rename(fullPath+randSuffix, fullPath)
	if err != nil {
		return nil, err
	}
	state := secretState{
		ContentHash: sha256.Sum256(secret.Content),
		Checksum:    secret.Checksum,
		FileInfo:    *filemode,
		Owner:       secret.Owner,
		Group:       secret.Group,
		Mode:        secret.Mode,
	}
	return &state, err
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
