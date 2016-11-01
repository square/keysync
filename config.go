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
	"io/ioutil"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// ClientConfig is the format of the values in the yaml
type ClientConfig struct {
	Mountpoint string `json:"mountpoint"` // Manditory: Where to mount
	Key        string `json:"key"`        // Manditory: Path to PEM key to use
	Cert       string `json:"cert"`       // Optional: PEM Certificate (If cert isn't in key file)
	User       string `json:"user"`       // Optional: User and Group are defaults for files without metadata
	Group      string `json:"group"`      // If unspecified, the global defaults are used.
}

// loadConfig looks in directory for files with suffix, and tries to load them
// as Yaml files describing clients for Keysync to load
// TODO: How do we represent errors opening some files, but success on others?
// We filter by prefix so we can keep configs and keys in the same directory
// To load a single file, provide the directory and its whole name as the suffix.
// TODO: If a file is provided instead of a folder, we should just load it as a config.
func loadConfig(directory, suffix *string) (map[string]ClientConfig, error) {
	files, err := ioutil.ReadDir(*directory)
	if err != nil {
		return nil, fmt.Errorf("Opening directory %s: %+v\n", directory, err)
	}
	configs := map[string]ClientConfig{}
	for _, file := range files {
		fileName := file.Name()
		if strings.HasSuffix(fileName, *suffix) {
			fmt.Println(fileName)
			// Read data into data
			data, err := ioutil.ReadFile(filepath.Join(*directory, fileName))
			if err != nil {
				// TODO: Do we just continue, instead of returning?
				return nil, fmt.Errorf("Opening %s: %+v\n", fileName, err)
			}
			var newConfigs map[string]ClientConfig
			err = yaml.Unmarshal(data, &newConfigs)
			if err != nil {
				return nil, fmt.Errorf("Parsing %s: %+v\n", fileName, err)
			}
			for name, config := range newConfigs {
				// TODO: Check if this is a duplicate.
				if config.Mountpoint == "" {
					return nil, fmt.Errorf("No mountpoint %s: %s", name, fileName)
				}
				if config.Key == "" {
					return nil, fmt.Errorf("No key %s: %s", name, fileName)
				}
				config.Key = resolvePath(*directory, config.Key)
				if config.Cert != "" {
					config.Cert = resolvePath(*directory, config.Cert)
				} else {
					// If no cert is provided, it's in the Key file.
					config.Cert = config.Key
				}
				config.Mountpoint = resolvePath(*directory, config.Mountpoint)
				configs[name] = config
			}
		}
	}
	return configs, nil
}

// resolvePath returns path if it's absolute, and joins it to directory otherwise.
func resolvePath(directory, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(directory, path)
}
