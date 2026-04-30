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
	// Source is the image source identifier (e.g. "xkcd", "oatmeal", "deviantart", "metmuseum", "tumblr").
	Source string `yaml:"source"`
	// KeepCount is the maximum number of image scheduler-managed images to retain (default: 1).
	KeepCount int `yaml:"keepCount"`
	// WhenUnmanaged controls behaviour when unmanaged images exist: "upload" (default), "skip", or "drain".
	WhenUnmanaged string `yaml:"whenUnmanaged"`
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

// DeviantArtFileConfig is the typed configuration for the deviantart source.
// It embeds all common scheduler fields and adds a strongly-typed Query field
// that is validated non-empty at load time.
type DeviantArtFileConfig struct {
	SchedulerFileConfig `yaml:",inline"`
	// Query is a DeviantArt search string, e.g. "boost:popular tag:lofi".
	Query string `yaml:"query"`
}

// MetMuseumFileConfig is the typed configuration for the metmuseum source.
// It embeds all common scheduler fields and adds an optional DepartmentIDs field.
// When DepartmentIDs is empty, all Met departments are searched.
type MetMuseumFileConfig struct {
	SchedulerFileConfig `yaml:",inline"`
	// DepartmentIDs restricts results to the given Met department IDs.
	// See https://collectionapi.metmuseum.org/public/collection/v1/departments for valid IDs.
	DepartmentIDs []int `yaml:"departmentIDs"`
}

// TumblrFileConfig is the typed configuration for the tumblr source.
// It embeds all common scheduler fields and adds a required Blog field.
type TumblrFileConfig struct {
	SchedulerFileConfig `yaml:",inline"`
	// Blog is the Tumblr blog name (e.g. "nasa"), without the .tumblr.com suffix.
	Blog string `yaml:"blog"`
}

// LoadSchedulerConfig reads and parses a YAML image scheduler config from the given path.
func LoadSchedulerConfig(path string) (*SchedulerFileConfig, error) {
	data, err := readConfigFile(path)
	if err != nil {
		return nil, err
	}

	var cfg SchedulerFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse image scheduler config %s: %w", path, err)
	}

	if err := applyDefaults(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadDeviantArtConfig reads and parses a YAML deviantart scheduler config from the given path.
// Returns an error if the required Query field is empty.
func LoadDeviantArtConfig(path string) (*DeviantArtFileConfig, error) {
	data, err := readConfigFile(path)
	if err != nil {
		return nil, err
	}

	var cfg DeviantArtFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse deviantart scheduler config %s: %w", path, err)
	}

	if err := applyDefaults(&cfg.SchedulerFileConfig); err != nil {
		return nil, err
	}
	if cfg.Query == "" {
		return nil, fmt.Errorf("deviantart scheduler config %s: query is required", path)
	}
	return &cfg, nil
}

// LoadMetMuseumConfig reads and parses a YAML metmuseum scheduler config from the given path.
func LoadMetMuseumConfig(path string) (*MetMuseumFileConfig, error) {
	data, err := readConfigFile(path)
	if err != nil {
		return nil, err
	}

	var cfg MetMuseumFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse metmuseum scheduler config %s: %w", path, err)
	}

	if err := applyDefaults(&cfg.SchedulerFileConfig); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadTumblrConfig reads and parses a YAML tumblr scheduler config from the given path.
// Returns an error if the required Blog field is empty.
func LoadTumblrConfig(path string) (*TumblrFileConfig, error) {
	data, err := readConfigFile(path)
	if err != nil {
		return nil, err
	}

	var cfg TumblrFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse tumblr scheduler config %s: %w", path, err)
	}

	if err := applyDefaults(&cfg.SchedulerFileConfig); err != nil {
		return nil, err
	}
	if cfg.Blog == "" {
		return nil, fmt.Errorf("tumblr scheduler config %s: blog is required", path)
	}
	return &cfg, nil
}

// PeekSource reads only the source field from a scheduler config file without full validation.
// Used by the binary entry point to determine which typed config loader to use.
func PeekSource(path string) (string, error) {
	data, err := readConfigFile(path)
	if err != nil {
		return "", err
	}
	var peek struct {
		Source string `yaml:"source"`
	}
	if err := yaml.Unmarshal(data, &peek); err != nil {
		return "", fmt.Errorf("failed to parse source field from %s: %w", path, err)
	}
	return peek.Source, nil
}

func readConfigFile(path string) ([]byte, error) {
	// #nosec G304 -- reading configuration from a user-provided path is intended
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read image scheduler config %s: %w", path, err)
	}
	return data, nil
}

func applyDefaults(cfg *SchedulerFileConfig) error {
	if cfg.KeepCount < 1 {
		cfg.KeepCount = 1
	}
	switch cfg.WhenUnmanaged {
	case "", "upload", "skip", "drain":
		// valid
	default:
		return fmt.Errorf("whenUnmanaged must be upload, skip, or drain (got %q)", cfg.WhenUnmanaged)
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	for i, cmd := range cfg.Commands {
		if cmd.Name == "" {
			return fmt.Errorf("command at index %d has empty name", i)
		}
	}
	return nil
}
