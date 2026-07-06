package integration

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the integration test configuration.
type Config struct {
	ServerURL string `yaml:"serverURL"`
}

var configPath = flag.String("config", "", "path to integration test config YAML (default: test/integration/local.yaml)")

// loadConfig reads the integration test config file.
// Returns nil if no config file is found or serverURL is empty — callers should skip.
func loadConfig() (*Config, error) {
	flag.Parse()

	path := *configPath
	if path == "" {
		path = os.Getenv("CONFIG_PATH")
	}
	if path == "" {
		path = "local.yaml"
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &cfg, nil
}
