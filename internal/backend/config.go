package backend

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type BackendConfig struct {
	Port int `yaml:"port"`
}

// LoadConfig loads configuration from the specified YAML file
func LoadConfig(configPath string) (*BackendConfig, error) {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse YAML
	var config BackendConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	return &config, nil
}
