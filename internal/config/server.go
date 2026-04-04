package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Database holds database connection configuration.
type Database struct {
	Type             string `yaml:"type"`
	ConnectionString string `yaml:"connectionString"`
	Namespace        string `yaml:"namespace"`
}

// ServiceConfig holds the full server configuration.
type ServiceConfig struct {
	Port                          int             `yaml:"port"`
	Database                      Database        `yaml:"database"`
	Commands                      []CommandConfig `yaml:"commands"`
	Timezone                      string          `yaml:"timezone"`
	ThumbnailWidth                int             `yaml:"thumbnailWidth"`
	LogLevel                      string          `yaml:"logLevel"`
	SvgFallbackLongSidePixelCount int             `yaml:"svgFallbackLongSidePixelCount"`
}

// LoadServerConfig reads and parses a YAML server config from the given path.
func LoadServerConfig(path string) (*ServiceConfig, error) {
	// #nosec G304 -- reading configuration from a user-provided path is intended; path is controlled via env/defaults
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config ServiceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	if err := validateCommandConfigs(config.Commands); err != nil {
		return nil, fmt.Errorf("invalid command configuration: %w", err)
	}

	// Defaults
	if config.Timezone == "" {
		config.Timezone = "UTC"
	}
	if config.ThumbnailWidth == 0 {
		config.ThumbnailWidth = 512
	}
	if config.SvgFallbackLongSidePixelCount <= 0 {
		config.SvgFallbackLongSidePixelCount = 4096
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	return &config, nil
}

// validateCommandConfigs ensures all command configurations have required and unique names.
func validateCommandConfigs(commands []CommandConfig) error {
	seenNames := make(map[string]bool, len(commands))
	for i, cmd := range commands {
		if cmd.Name == "" {
			return fmt.Errorf("command at index %d has empty name", i)
		}
		if seenNames[cmd.Name] {
			return fmt.Errorf("duplicate command name: %s", cmd.Name)
		}
		seenNames[cmd.Name] = true
	}
	return nil
}
