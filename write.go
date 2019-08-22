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
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/square/keysync/output"
	"github.com/square/keysync/ownership"

	"github.com/sirupsen/logrus"
)

// OutputCollection handles a collection of outputs.
type OutputCollection interface {
	NewOutput(clientConfig ClientConfig, logger *logrus.Entry) (Output, error)
	// Cleanup unknown clients (eg, ones deleted while keysync was not running)
	// Returns a count of deleted clients
	Cleanup(map[string]struct{}, *logrus.Entry) (uint, []error)
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
	// Returns a count of deleted files
	RemoveAll() (uint, error)
	// Cleanup unknown files (eg, ones deleted in Keywhiz while keysync was not running)
	// Returns a count of deleted files
	Cleanup(map[string]Secret) (uint, error)
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

func (c OutputDirCollection) Cleanup(known map[string]struct{}, logger *logrus.Entry) (uint, []error) {
	var errors []error
	var deleted uint = 0

	fileInfos, err := ioutil.ReadDir(c.Config.SecretsDir)
	if err != nil {
		errors = append(errors, err)
		logger.WithError(err).WithField("SecretsDir", c.Config.SecretsDir).Warn("Couldn't read secrets dir")
	}
	for _, fileInfo := range fileInfos {
		logger := logger.WithField("name", fileInfo.Name())
		if !fileInfo.IsDir() {
			// Keysync won't have written a file here, so safest to not touch it
			logger.Warn("Found unknown file, ignoring")
			continue
		}
		if _, present := known[fileInfo.Name()]; !present {
			logger.Info("Deleting unknown directory")
			if err := os.RemoveAll(filepath.Join(c.Config.SecretsDir, fileInfo.Name())); err != nil {
				logger.WithError(err).Warn("Error removing unknown directory")
				errors = append(errors, err)
			}
			// os.RemoveAll may have returned an error but partially removed files, so increment
			// deleted despite the error, so we know changes may have been made.
			deleted++
		}
	}

	return deleted, errors
}

// OutputDir implements Output to files, which is the typical keysync usage to a tmpfs.
type OutputDir struct {
	WriteDirectory    string
	DefaultOwnership  ownership.Ownership
	EnforceFilesystem output.Filesystem // What filesystem type do we expect to write to?
	ChownFiles        bool              // Do we chown the file? (Needs root or CAP_CHOWN).
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
	fileinfo, err := output.GetFileInfo(f)
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

func (out *OutputDir) RemoveAll() (uint, error) {
	// TODO: This count isn't accurate, but it also isn't worth reimplementing os.RemoveAll to count
	return 1, os.RemoveAll(out.WriteDirectory)
}

func (out *OutputDir) Cleanup(secrets map[string]Secret) (uint, error) {
	var deleted uint

	fileInfos, err := ioutil.ReadDir(out.WriteDirectory)
	if err != nil {
		return deleted, fmt.Errorf("couldn't read directory: %s", out.WriteDirectory)
	}
	for _, fileInfo := range fileInfos {
		existingFile := fileInfo.Name()
		if _, present := secrets[existingFile]; !present {
			// This file wasn't written in the loop above, so we remove it.
			out.Logger.WithField("file", existingFile).Info("Removing unknown file")
			err := os.Remove(filepath.Join(out.WriteDirectory, existingFile))
			if err != nil {
				// Not fatal, so log and continue.
				out.Logger.WithError(err).Warnf("Unable to delete file")
			} else {
				deleted++
			}
		}
	}
	return deleted, nil
}

// Write puts a Secret into OutputDir
func (out *OutputDir) Write(secret *Secret) (*secretState, error) {

	filename, err := secret.Filename()
	if err != nil {
		return nil, err
	}

	mode, err := secret.ModeValue()
	if err != nil {
		return nil, err
	}
	fileInfo := output.FileInfo{Mode: mode}
	if out.ChownFiles {
		owner := secret.OwnershipValue(out.DefaultOwnership)
		fileInfo.UID = owner.UID
		fileInfo.GID = owner.GID
	}
	path := filepath.Join(out.WriteDirectory, filename)
	fileinfo, err := output.WriteFileAtomically(path, out.ChownFiles, fileInfo, out.EnforceFilesystem, secret.Content)
	if err != nil {
		return nil, err
	}

	state := secretState{
		ContentHash: sha256.Sum256(secret.Content),
		Checksum:    secret.Checksum,
		FileInfo:    *fileinfo,
		Owner:       secret.Owner,
		Group:       secret.Group,
		Mode:        secret.Mode,
	}
	return &state, err
}
