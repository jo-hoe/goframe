package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SchedulerFileConfig is the YAML representation of image scheduler configuration.
// It is loaded from a config file and converted to a SchedulerConfig for use with RunOnce.
type SchedulerFileConfig struct {
	// GoframeURL is the base URL of the goframe service.
	GoframeURL string `yaml:"goframeURL"`
	// SourceName is the unique identity of this image scheduler instance.
	SourceName string `yaml:"sourceName"`
	// KeepCount is the maximum number of image scheduler-managed images to retain (default: 1).
	KeepCount int `yaml:"keepCount"`
	// SkipIfUnmanagedImagesExceed is the max number of images not owned by this scheduler
	// that still allows the scheduler to act (default: 0, meaning skip if any unmanaged image exists).
	SkipIfUnmanagedImagesExceed int `yaml:"skipIfUnmanagedImagesExceed"`
	// LogLevel controls verbosity (debug, info, warn, error).
	LogLevel string `yaml:"logLevel"`
	// Sources lists the available image sources and whether each is enabled.
	Sources SchedulerSources `yaml:"sources"`
	// Commands is an optional processing pipeline applied to each fetched image before upload.
	Commands []CommandConfig `yaml:"commands"`
}

// SchedulerSources holds per-source enable flags.
type SchedulerSources struct {
	XKCD SchedulerSource `yaml:"xkcd"`
}

// SchedulerSource holds the enabled flag for a single image source.
type SchedulerSource struct {
	Enabled bool `yaml:"enabled"`
}

// LoadSchedulerConfig reads and parses a YAML image scheduler config from the given path.
func LoadSchedulerConfig(path string) (*SchedulerFileConfig, error) {
	// #nosec G304 -- reading configuration from a user-provided path is intended
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read image scheduler config %s: %w", path, err)
	}

	var cfg SchedulerFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse image scheduler config %s: %w", path, err)
	}

	if cfg.KeepCount < 1 {
		cfg.KeepCount = 1
	}
	if cfg.SkipIfUnmanagedImagesExceed < 0 {
		cfg.SkipIfUnmanagedImagesExceed = 0
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	for i, cmd := range cfg.Commands {
		if cmd.Name == "" {
			return nil, fmt.Errorf("command at index %d has empty name", i)
		}
	}

	return &cfg, nil
}
