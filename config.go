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

package keysync

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config is the main yaml configuration file passed to the keysync binary
type Config struct {
	ClientsDir    string     `yaml:"client_directory"`  // A directory of configuration files
	SecretsDir    string     `yaml:"secrets_directory"` // The directory secrets will be written to
	CaFile        string     `yaml:"ca_file"`           // The CA to trust (PEM) for Keywhiz communication
	YamlExt       string     `yaml:"yaml_ext"`          // The filename extension of the yaml config files
	PollInterval  string     `yaml:"poll_interval"`     // If specified, poll at the given interval, otherwise, exit after syncing
	ClientTimeout string     `yaml:"client_timeout"`    // If specified, timeout client connections after specified duration, otherwise use default.
	MinBackoff    string     `yaml:"min_backoff"`       // If specified, wait time before first retry, otherwise, use default.
	MaxBackoff    string     `yaml:"max_backoff"`       // If specified, max wait time before retries, otherwise, use default.
	MaxRetries    uint16     `yaml:"max_retries"`       // If specified, retry each HTTP call after non-200 response
	Server        string     `yaml:"server"`            // The server to connect to (host:port)
	Debug         bool       `yaml:"debug"`             // Enable debugging output
	DefaultUser   string     `yaml:"default_user"`      // Default user to own files
	DefaultGroup  string     `yaml:"default_group"`     // Default group to own files
	PasswdFile    string     `yaml:"passwd_file"`       // /etc/passwd, for uid lookups
	GroupFile     string     `yaml:"group_file"`        // /etc/groups, for gid lookups
	APIPort       uint16     `yaml:"api_port"`          // Port for API to listen on
	SentryDSN     string     `yaml:"sentry_dsn"`        // Sentry DSN
	SentryCaFile  string     `yaml:"sentry_ca_file"`    // The CA to trust (PEM) for Sentry communication
	FsType        Filesystem `yaml:"filesystem_type"`   // Enforce writing this type of filesystem. Use value from statfs.
	ChownFiles    bool       `yaml:"chown_files"`       // Do we chown files? Set to false when running without CAP_CHOWN.
	MetricsPrefix string     `yaml:"metrics_prefix"`    // Prefix metric names with this
}

// The ClientConfig describes a single Keywhiz client.  There are typically many of these per keysync instance.
type ClientConfig struct {
	Key        string `yaml:"key"`       // Mandatory: Path to PEM key to use
	Cert       string `yaml:"cert"`      // Optional: PEM Certificate (If cert isn't in key file)
	User       string `yaml:"user"`      // Optional: User and Group are defaults for files without metadata
	DirName    string `yaml:"directory"` // Optional: What directory under SecretsDir this client is in. Defaults to the client name.
	Group      string `yaml:"group"`     // Optional: If unspecified, the global defaults are used.
	MaxRetries uint16
	Timeout    string
	MinBackoff string
	MaxBackoff string
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

	if config.SecretsDir == "" {
		return nil, fmt.Errorf("Mandatory config secrets_directory not provided: %s", configFile)
	}

	if config.PasswdFile == "" {
		config.PasswdFile = "/etc/passwd"
	}

	if config.GroupFile == "" {
		config.GroupFile = "/etc/group"
	}

	if config.MaxRetries < 1 {
		config.MaxRetries = 1
	}

	if config.ClientTimeout == "" {
		config.ClientTimeout = "60s"
	}

	if config.MinBackoff == "" {
		config.MinBackoff = "100ms"
	}

	if config.MaxBackoff == "" {
		config.MaxBackoff = "10s"
	}

	return &config, nil
}

// LoadClients looks in directory for files with suffix, and tries to load them
// as Yaml files describing clients for Keysync to load
// We filter by the yaml extension so we can keep configs and keys in the same directory
func (config *Config) LoadClients() (map[string]ClientConfig, error) {
	files, err := ioutil.ReadDir(config.ClientsDir)
	if err != nil {
		return nil, fmt.Errorf("Failed opening directory %s: %+v\n", config.ClientsDir, err)
	}
	configs := map[string]ClientConfig{}
	for _, file := range files {
		fileName := file.Name()
		if strings.HasSuffix(fileName, config.YamlExt) {
			// Read data into data
			data, err := ioutil.ReadFile(filepath.Join(config.ClientsDir, fileName))
			if err != nil {
				return nil, fmt.Errorf("Failed opening %s: %+v\n", fileName, err)
			}
			var newClients map[string]ClientConfig
			err = yaml.Unmarshal(data, &newClients)
			if err != nil {
				return nil, fmt.Errorf("Failed parsing %s: %+v\n", fileName, err)
			}
			for name, client := range newClients {
				// TODO: Check if this is a duplicate.
				if client.DirName == "" {
					client.DirName = name
				}

				client.setDefaults(config)
				if err := client.validate(); err != nil {
					return nil, fmt.Errorf("Failed validating %s: %+v\n", fileName, err)
				}
				client.resolveKeyPair(config)

				configs[name] = client
			}
		}
	}
	return configs, nil
}

func (c *ClientConfig) setDefaults(cfg *Config) {
	c.MinBackoff = cfg.MinBackoff
	c.MaxBackoff = cfg.MaxBackoff
	c.MaxRetries = cfg.MaxRetries
	c.Timeout = cfg.ClientTimeout
}

func (c *ClientConfig) validate() error {
	if c.Key == "" {
		return errors.New("No key in config")
	}

	return nil
}

func (c *ClientConfig) resolveKeyPair(cfg *Config) {
	c.Key = resolvePath(cfg.ClientsDir, c.Key)
	if c.Cert != "" {
		c.Cert = resolvePath(cfg.ClientsDir, c.Cert)
	} else {
		// If no cert is provided, it's in the Key file.
		c.Cert = c.Key
	}
}

// resolvePath returns path if it's absolute, and joins it to directory otherwise.
func resolvePath(directory, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(directory, path)
}
