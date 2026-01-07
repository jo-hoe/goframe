package backend

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ProcessorConfig represents a generic processor configuration
type ProcessorConfig struct {
	Name   string         `yaml:"name"`
	Params map[string]any `yaml:",inline"`
}

type Database struct {
	Type             string
	ConnectionString string
}

type BackendConfig struct {
	Port            int               `yaml:"port"`
	Database        Database          `yaml:"database"`
	ImageTargetType string            `yaml:"imageTargetType"`
	Processors      []ProcessorConfig `yaml:"processors"`
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

	// Validate processors
	if err := validateProcessors(config.Processors); err != nil {
		return nil, fmt.Errorf("invalid processor configuration: %w", err)
	}

	return &config, nil
}

// validateProcessors ensures all processor configurations have required fields
func validateProcessors(processors []ProcessorConfig) error {
	seenNames := make(map[string]bool)

	for i, proc := range processors {
		// Validate name is not empty
		if proc.Name == "" {
			return fmt.Errorf("processor at index %d has empty name", i)
		}

		// Validate name is unique
		if seenNames[proc.Name] {
			return fmt.Errorf("duplicate processor name: %s", proc.Name)
		}
		seenNames[proc.Name] = true
	}

	return nil
}
