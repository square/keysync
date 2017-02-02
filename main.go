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
	"fmt"
	"os"
	"time"

	"log"
	"net/http"

	raven "github.com/getsentry/raven-go"
	"github.com/rcrowley/go-metrics"
	"github.com/square/go-sq-metrics"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var (
		app        = kingpin.New("keysync", "A client for Keywhiz")
		configFile = app.Flag("config", "The base YAML configuration file").PlaceHolder("config.yaml").Required().String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	fmt.Printf("Loading config: %s\n", *configFile)

	config, err := LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Couldn't load configuration: %v", err)
	}

	// If not set in the config, raven will also use the SENTRY_DSN environment variable
	if config.SentryDSN != "" {
		raven.SetDSN(config.SentryDSN)
	}

	raven.CapturePanicAndWait(func() {
		metricsHandle := sqmetrics.NewMetrics(config.MetricsURL, config.MetricsPrefix, http.DefaultClient, 30*time.Second, metrics.DefaultRegistry, &log.Logger{})

		syncer := NewSyncer(config, metricsHandle)

		// Start the API server
		if config.APIPort != 0 {
			NewAPIServer(syncer, config.APIPort)
		}

		err := syncer.Run()
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
		}
	}, nil)
}
