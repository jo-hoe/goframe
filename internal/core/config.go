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
	Port                          int             `yaml:"port"`
	Database                      Database        `yaml:"database"`
	Commands                      []CommandConfig `yaml:"commands"`
	RotationTimezone              string          `yaml:"rotationTimezone"`
	ThumbnailWidth                int             `yaml:"thumbnailWidth"`
	LogLevel                      string          `yaml:"logLevel"`
	SvgFallbackLongSidePixelCount int             `yaml:"svgFallbackLongSidePixelCount"`
}

// LoadConfig loads configuration from the specified YAML file
func LoadConfig(configPath string) (*ServiceConfig, error) {
	// Read the config file
	// #nosec G304 -- reading configuration from a user-provided path is intended; path is controlled via env/defaults
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

	// Defaults
	if config.RotationTimezone == "" {
		config.RotationTimezone = "UTC"
	}
	if config.ThumbnailWidth == 0 {
		config.ThumbnailWidth = 512
	}
	// Default long-side pixel count for SVGs without explicit size
	if config.SvgFallbackLongSidePixelCount <= 0 {
		config.SvgFallbackLongSidePixelCount = 4096
	}
	// Default log level
	if config.LogLevel == "" {
		config.LogLevel = "info"
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
