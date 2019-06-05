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

package main

import (
	"fmt"
	"os"

	"github.com/square/keysync"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var (
		app        = kingpin.New("keysync-monitor", "Health check/monitor for keysync")
		configFile = app.Flag("config", "The base YAML configuration file").PlaceHolder("config.yaml").Required().String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	config, err := keysync.LoadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed loading configuration: %s\n", err)
		os.Exit(1)
	}

	checks := []func(*keysync.Config) []error{
		checkPaths,
		checkServerHealth,
		checkClientHealth,
		checkDiskUsage,
	}

	errs := runChecks(config, checks)
	if len(errs) > 0 {
		printErrors(errs)
		os.Exit(1)
	}
}

func runChecks(config *keysync.Config, checks []func(*keysync.Config) []error) []error {
	errs := []error{}
	for _, check := range checks {
		e := check(config)
		if len(e) > 0 {
			errs = append(errs, e...)
			return errs
		}
	}
	return errs
}

func printErrors(errs []error) {
	fmt.Fprintf(os.Stderr, "found the following problems:\n")
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "- %s\n", err)
	}
}
