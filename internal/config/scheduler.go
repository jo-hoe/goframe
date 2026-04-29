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
	// Source is the image source identifier (e.g. "xkcd", "pusheen", "oatmeal").
	Source string `yaml:"source"`
	// KeepCount is the maximum number of image scheduler-managed images to retain (default: 1).
	KeepCount int `yaml:"keepCount"`
	// ExclusionGroup is an optional group name. When set, a successful upload causes all images
	// owned by other members of the same group to be deleted.
	ExclusionGroup string `yaml:"exclusionGroup"`
	// GroupMembers lists the source names of all schedulers in the same ExclusionGroup,
	// including this scheduler's own SourceName. Populated by the operator at config-render time.
	GroupMembers []string `yaml:"groupMembers"`
	// LogLevel controls verbosity (debug, info, warn, error).
	LogLevel string `yaml:"logLevel"`
	// Commands is an optional processing pipeline applied to each fetched image before upload.
	Commands []CommandConfig `yaml:"commands"`
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
