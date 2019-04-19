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

// This is the main entry point for Keysync.  It assumes a bit more about the
// environment you're using keysync in than the keysync library.  In
// particular, you may want to have your own version of this for a different
// monitoring system than Sentry, a different configuration or command line
// format, or any other customization you need.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"os"
	"time"

	"github.com/square/keysync"

	"github.com/evalphobia/logrus_sentry"
	"github.com/getsentry/raven-go"
	"github.com/rcrowley/go-metrics"
	"github.com/sirupsen/logrus"
	sqmetrics "github.com/square/go-sq-metrics"
	"gopkg.in/alecthomas/kingpin.v2"
)

var log = logrus.New()

// Release is passed to Sentry as the release.  It is deliberately unset here
// so that it can be set with the -X argument to the go linker.
var release string

func main() {
	var (
		app        = kingpin.New("keysync", "A client for Keywhiz")
		configFile = app.Flag("config", "The base YAML configuration file").PlaceHolder("config.yaml").Required().String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	hostname, err := os.Hostname()
	if err != nil {
		log.WithError(err).Error("Failed resolving hostname")
		hostname = "unknown"
	}
	logger := log.WithFields(logrus.Fields{
		// https://github.com/evalphobia/logrus_sentry#special-fields
		"server_name": hostname,
	})
	if release == "" {
		release = "(version not set)"
	}
	logger.WithField("release", release).Info("Keysync starting")

	logger.WithField("file", *configFile).Info("Loading config")
	config, err := keysync.LoadConfig(*configFile)
	if err != nil {
		logger.WithError(err).Fatal("Failed loading configuration")
	}
	config.WithLogger(logger)

	if config.SentryDSN != "" {
		hook, err := configureLogrusSentry(config.SentryDSN, config.SentryCaFile)

		if err == nil {
			log.Hooks.Add(hook)
			logger.Debug("Logrus Sentry hook added")
		} else {
			logger.WithError(err).Error("Failed loading Sentry hook")
		}
	}

	captured, errorId := raven.CapturePanicAndWait(func() {
		metricsHandle := sqmetrics.NewMetrics("", config.MetricsPrefix, http.DefaultClient, 1*time.Second, metrics.DefaultRegistry, &stdlog.Logger{})

		syncer, err := keysync.NewSyncer(config, keysync.OutputDirCollection{config}, logger, metricsHandle)
		if err != nil {
			logger.WithError(err).Fatal("Failed while creating syncer")
		}

		// Start the API server
		if config.APIPort != 0 {
			keysync.NewAPIServer(syncer, config.APIPort, logger, metricsHandle)
		}

		logger.Info("Starting syncer")
		err = syncer.Run()
		if err != nil {
			logger.WithError(err).Fatal("Failed while running syncer")
		}
	}, nil)
	if captured != nil {
		logger.Info("Panic errorId: %s", errorId)
		panic(captured)
	} else {
		logger.Info("Exiting normally")
	}
}

// This is modified from raven.newTransport()
func newTransport(CaFile string) (raven.Transport, error) {
	t := &raven.HTTPTransport{}

	transport := http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if CaFile != "" {
		b, err := ioutil.ReadFile(CaFile)
		if err != nil {
			return t, err
		}
		rootCAs := x509.NewCertPool()
		ok := rootCAs.AppendCertsFromPEM(b)
		if !ok {
			return t, errors.New("failed to load root CAs")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: rootCAs}
	}

	t.Client = &http.Client{
		Transport: &transport,
	}
	return t, nil
}

func configureLogrusSentry(DSN, CaFile string) (*logrus_sentry.SentryHook, error) {
	// If a custom CaFile is set, create a custom transport
	var transport raven.Transport
	var err error
	transport, err = newTransport(CaFile)
	if err != nil {
		return nil, err
	}

	client, err := raven.New(DSN)
	if err != nil {
		return nil, err
	}

	client.SetRelease(release)

	client.Transport = transport

	// Sentry on the configured logrus levels:
	hook, err := logrus_sentry.NewWithClientSentryHook(client, []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
	})
	hook.StacktraceConfiguration.Enable = true
	hook.Timeout = 1 * time.Second

	return hook, err
}
