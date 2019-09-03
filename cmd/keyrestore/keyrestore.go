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

// This is the main entry point for Keysync.  It assumes a bit more about the environment you're using keysync in than
// the keysync library.  In particular, you may want to have your own version of this for a different monitoring system
// than Sentry, a different configuration or command line format, or any other customization you need.
package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/square/keysync/backup"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/square/keysync"
)

var log = logrus.New()

func main() {
	var (
		app        = kingpin.New("keyrestore", "Unpack and install a Keysync backup")
		configFile = app.Flag("config", "The base YAML configuration file").PlaceHolder("config.yaml").Required().ExistingFile()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	logger := log.WithFields(logrus.Fields{})
	logger.WithField("file", *configFile).Infof("Loading config")

	config, err := keysync.LoadConfig(*configFile)
	if err != nil {
		logger.WithError(err).Fatal("Failed loading configuration")
	}

	bup := backup.FileBackup{
		SecretsDirectory: config.SecretsDir,
		BackupPath:       config.BackupPath,
		KeyPath:          config.BackupKeyPath,
		Chown:            config.ChownFiles,
		EnforceFS:        config.FsType,
	}

	logger.Info("Restoring backup")
	if err := bup.Restore(); err != nil {
		logger.WithError(err).Warn("error restoring backup")
	} else {
		logger.Info("Backup restored")
	}
}
