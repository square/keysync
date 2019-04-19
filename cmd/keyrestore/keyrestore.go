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
	stdlog "log"
	"net/http"
	"os"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/sirupsen/logrus"
	sqmetrics "github.com/square/go-sq-metrics"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/square/keysync"
)

var log = logrus.New()

func main() {
	var (
		app        = kingpin.New("keyrestore", "Unpack and install a Keywhiz backup bundle")
		configFile = app.Flag("config", "The base YAML configuration file").PlaceHolder("config.yaml").Required().ExistingFile()
		user       = app.Flag("user", "Default user to install files as (unless overridden in bundle)").PlaceHolder("USER").Required().String()
		group      = app.Flag("group", "Default group to install files as (unless overridden in bundle)").PlaceHolder("GROUP").Required().String()
		dirName    = app.Flag("dir-name", "Directory (under the global secrets directory) to install files into").PlaceHolder("NAME").Required().String()
		bundleFile = app.Arg("bundle", "Keywhiz backup bundle (JSON)").Required().ExistingFile()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	logger := log.WithFields(logrus.Fields{})
	logger.WithField("file", *configFile).Infof("Loading config")

	config, err := keysync.LoadConfig(*configFile)
	if err != nil {
		logger.WithError(err).Fatal("Failed loading configuration")
	}
	config.WithLogger(logger)

	clientConfig := keysync.ClientConfig{
		User:    *user,
		Group:   *group,
		DirName: *dirName,
	}

	metricsHandle := sqmetrics.NewMetrics("", config.MetricsPrefix, http.DefaultClient, 1*time.Second, metrics.DefaultRegistry, &stdlog.Logger{})

	syncer, err := keysync.NewSyncerFromFile(config, clientConfig, *bundleFile, logger, metricsHandle)
	if err != nil {
		logger.WithError(err).Fatal("Failed while creating syncer")
	}

	errs := syncer.RunOnce()
	if len(errs) > 0 {
		logger.WithError(errs[0]).Fatalf("Failed while running syncer: %v", errs)
		os.Exit(1)
	}
}
