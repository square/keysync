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
	stdlog "log"
	"net/http"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/evalphobia/logrus_sentry"
	raven "github.com/getsentry/raven-go"
	"github.com/rcrowley/go-metrics"
	"github.com/square/go-sq-metrics"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/square/keysync"
)

var log = logrus.New()

func main() {
	var (
		app        = kingpin.New("keysync", "A client for Keywhiz")
		configFile = app.Flag("config", "The base YAML configuration file").PlaceHolder("config.yaml").Required().String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	hostname, err := os.Hostname()
	if err != nil {
		log.WithError(err).Error("Resolving hostname")
		hostname = "unknown"
	}
	logger := log.WithFields(logrus.Fields{
		// https://github.com/evalphobia/logrus_sentry#special-fields
		"server_name": hostname,
	})
	logger.WithField("file", *configFile).Infof("Loading config")

	config, err := keysync.LoadConfig(*configFile)

	if err != nil {
		logger.WithError(err).Fatal("Couldn't load configuration")
	}

	// If not set in the config, raven will also use the SENTRY_DSN environment variable
	if config.SentryDSN != "" {
		hook, err := logrus_sentry.NewSentryHook(config.SentryDSN, []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
		})

		if err == nil {
			log.Hooks.Add(hook)
			logger.Debug("Logrus Sentry hook added")
		} else {
			logger.WithError(err).Error("Logrus Sentry hook")
		}
	}

	raven.CapturePanicAndWait(func() {
		metricsHandle := sqmetrics.NewMetrics(config.MetricsURL, config.MetricsPrefix, http.DefaultClient, 30*time.Second, metrics.DefaultRegistry, &stdlog.Logger{})

		syncer, err := keysync.NewSyncer(config, logger, metricsHandle)
		if err != nil {
			logger.WithError(err).Fatal("Failed while creating syncer")
		}

		// Start the API server
		if config.APIPort != 0 {
			keysync.NewAPIServer(syncer, config.APIPort, logger)
		}

		err = syncer.Run()
		if err != nil {
			logger.WithError(err).Fatal("Failed while running syncer")
		}
	}, nil)
}
