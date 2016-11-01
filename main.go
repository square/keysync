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
	"net/url"

	"github.com/rcrowley/go-metrics"
	"github.com/square/go-sq-metrics"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var (
		app          = kingpin.New("keysync", "A client for Keywhiz")
		configDir    = app.Flag("config", "A directory of configuration files").PlaceHolder("DIR").Required().String()
		caFile       = app.Flag("ca", "The CA to trust (PEM)").PlaceHolder("cacert.pem").Required().String()
		yamlExt      = app.Flag("extension", "The filename extension of the yaml config files").Default(".yaml").String()
		pollInterval = app.Flag("interval", "The interval to poll at").Default("30s").Duration()
		server       = app.Flag("server", "The to connect to").PlaceHolder("hostname:port").Required().String()
		debug        = app.Flag("debug", "Enable debugging output").Default("false").Bool()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	fmt.Printf("Directory: %s\n", *configDir)
	fmt.Printf("Polling at: %v\n", *pollInterval)

	configs, err := loadConfig(configDir, yamlExt)
	if err != nil {
		fmt.Printf("Error loading config: %+v\n", err)
		return
	}

	metricsHandle := sqmetrics.NewMetrics("", "TODO:Hostname", http.DefaultClient, 30*time.Second, metrics.DefaultRegistry, &log.Logger{})

	serverURL, err := url.Parse("https://" + *server)
	if err != nil {
		fmt.Printf("Error parsing url %s: https://%s, %+v\n", *server, err)
		return
	}

	syncer := NewSyncer(configs, serverURL, caFile, *debug, metricsHandle)

	syncer.Run()
}
