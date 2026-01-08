package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CommandConfig represents a generic command configuration
type CommandConfig struct {
	Name   string         `yaml:"name"`
	Params map[string]any `yaml:",inline"`
}

type Database struct {
	Type             string `yaml:"type"`
	ConnectionString string `yaml:"connectionString"`
}

type ServiceConfig struct {
	Port     int             `yaml:"port"`
	Database Database        `yaml:"database"`
	Commands []CommandConfig `yaml:"commands"`
}

// LoadConfig loads configuration from the specified YAML file
func LoadConfig(configPath string) (*ServiceConfig, error) {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse YAML
	var config ServiceConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Validate commands
	if err := validateCommands(config.Commands); err != nil {
		return nil, fmt.Errorf("invalid command configuration: %w", err)
	}

	return &config, nil
}

// validateCommands ensures all command configurations have required fields
func validateCommands(commands []CommandConfig) error {
	seenNames := make(map[string]bool)

	for i, cmd := range commands {
		// Validate name is not empty
		if cmd.Name == "" {
			return fmt.Errorf("command at index %d has empty name", i)
		}

		// Validate name is unique
		if seenNames[cmd.Name] {
			return fmt.Errorf("duplicate command name: %s", cmd.Name)
		}
		seenNames[cmd.Name] = true
	}

	return nil
}
