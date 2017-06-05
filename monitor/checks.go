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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/square/keysync"
)

func checkPaths(config *keysync.Config) []error {
	errs := []error{}
	errs = append(errs, directoryExists(config.SecretsDir)...)
	errs = append(errs, directoryExists(config.ClientsDir)...)
	errs = append(errs, fileExists(config.CaFile)...)
	return errs
}

func checkServerHealth(config *keysync.Config) []error {
	url := fmt.Sprintf("http://localhost:%d/status", config.APIPort)

	resp, err := http.Get(url)
	if err != nil {
		return []error{fmt.Errorf("unable to talk to status: %s", err)}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []error{fmt.Errorf("unable to talk to status: %s", err)}
	}

	status := &keysync.StatusResponse{}
	err = json.Unmarshal(body, &status)
	if err != nil {
		return []error{fmt.Errorf("invalid JSON status response: %s", err)}
	}

	if !status.Ok {
		return []error{fmt.Errorf("keysync unhealthy: %s", status.Message)}
	}

	return nil
}

func fileExists(path string) []error {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return []error{fmt.Errorf("expected '%s' to be a file", path)}
	}
	return nil
}

func directoryExists(path string) []error {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return []error{fmt.Errorf("expected '%s' to be a directory", path)}
	}
	return nil
}
