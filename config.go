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

// Config is the main yaml configuration file passed to the keysync binary
type Config struct {
	ClientsDir   string     `json:"client_directory"` // A directory of configuration files
	CaFile       string     `json:"ca_file"`          // The CA to trust (PEM)
	YamlExt      string     `json:"yaml_ext"`         // The filename extension of the yaml config files
	PollInterval string     `json:"yaml_ext"`         // If specified, poll at the given interval, otherwise, exit after syncing
	Server       string     `json:"server"`           // The server to connect to (host:port)
	Debug        bool       `json:"debug"`            // Enable debugging output
	DefaultUser  string     `json:"default_user"`     // Default user to own files
	DefaultGroup string     `json:"default_group"`    // Default group to own files
	APIPort      uint16     `json:"api_port"`         // Port for API to listen on
	SentryDSN    string     `json:"sentry_dsn"`       // Sentry DSN
	FsType       Filesystem `json:"filesystem_type"`  // Enforce writing this type of filesystem. Use value from statfs.
}

// The ClientConfig describes a single Keywhiz client.  There are typically many of these per keysync instance.
type ClientConfig struct {
	Mountpoint string `json:"mountpoint"` // Mandatory: Where to mount
	Key        string `json:"key"`        // Mandatory: Path to PEM key to use
	Cert       string `json:"cert"`       // Optional: PEM Certificate (If cert isn't in key file)
	User       string `json:"user"`       // Optional: User and Group are defaults for files without metadata
	Group      string `json:"group"`      // If unspecified, the global defaults are used.
}

// LoadConfig loads the "global" keysync configuration file.  This would generally be called on startup.
func LoadConfig(configFile string) (*Config, error) {
	var config Config
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("Loading config %s: %v", configFile, err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("Parsing config file: %v", err)
	}

	// TODO: Apply any defaults or validation.

	return &config, nil
}

// LoadClients looks in directory for files with suffix, and tries to load them
// as Yaml files describing clients for Keysync to load
// We filter by the yaml extension so we can keep configs and keys in the same directory
func (config *Config) LoadClients() (map[string]ClientConfig, error) {
	files, err := ioutil.ReadDir(config.ClientsDir)
	if err != nil {
		return nil, fmt.Errorf("Opening directory %s: %+v\n", config.ClientsDir, err)
	}
	configs := map[string]ClientConfig{}
	for _, file := range files {
		fileName := file.Name()
		if strings.HasSuffix(fileName, config.YamlExt) {
			fmt.Println(fileName)
			// Read data into data
			data, err := ioutil.ReadFile(filepath.Join(config.ClientsDir, fileName))
			if err != nil {
				return nil, fmt.Errorf("Opening %s: %+v\n", fileName, err)
			}
			var newClients map[string]ClientConfig
			err = yaml.Unmarshal(data, &newClients)
			if err != nil {
				return nil, fmt.Errorf("Parsing %s: %+v\n", fileName, err)
			}
			for name, client := range newClients {
				// TODO: Check if this is a duplicate.
				if client.Mountpoint == "" {
					return nil, fmt.Errorf("No mountpoint %s: %s", name, fileName)
				}
				if client.Key == "" {
					return nil, fmt.Errorf("No key %s: %s", name, fileName)
				}
				client.Key = resolvePath(config.ClientsDir, client.Key)
				if client.Cert != "" {
					client.Cert = resolvePath(config.ClientsDir, client.Cert)
				} else {
					// If no cert is provided, it's in the Key file.
					client.Cert = client.Key
				}
				client.Mountpoint = resolvePath(config.ClientsDir, client.Mountpoint)
				configs[name] = client
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
