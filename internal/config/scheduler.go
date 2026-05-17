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
	// Source is the image source identifier (e.g. "xkcd", "oatmeal", "metmuseum", "tumblr", "s3").
	Source string `yaml:"source"`
	// Group is an optional group name shared by schedulers that are mutually exclusive.
	// When a scheduler in a group uploads, all images owned by other group members are deleted.
	Group string `yaml:"group"`
	// GroupMembers lists the source names of all schedulers sharing Group, including self.
	// Populated by the operator; not set by users directly.
	GroupMembers []string `yaml:"groupMembers"`
	// OnExternalImages controls what happens when external images exist (images not owned
	// by this scheduler or any group member). Values: "ignore" (default), "takeover", "yield".
	OnExternalImages string `yaml:"onExternalImages"`
	// LogLevel controls verbosity (debug, info, warn, error).
	LogLevel string `yaml:"logLevel"`
	// Commands is an optional processing pipeline applied to each fetched image before upload.
	Commands []CommandConfig `yaml:"commands"`
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
// It embeds all common scheduler fields and adds a required Blogs field.
type TumblrFileConfig struct {
	SchedulerFileConfig `yaml:",inline"`
	// Blogs is the list of Tumblr blog names (e.g. ["nasa", "pusheen"]), without the .tumblr.com suffix.
	// One blog is picked randomly per run.
	Blogs []string `yaml:"blogs"`
}

// S3FileConfig is the typed configuration for the s3 source.
// Compatible with AWS S3, RustFS, MinIO, and any S3-compatible storage.
type S3FileConfig struct {
	SchedulerFileConfig `yaml:",inline"`
	// Endpoint is the base URL of the S3-compatible service (no trailing slash, no bucket).
	// For AWS S3 use "https://s3.<region>.amazonaws.com".
	// For RustFS/MinIO use e.g. "http://rustfs:9000".
	Endpoint string `yaml:"endpoint"`
	// Bucket is the name of the S3 bucket to fetch images from.
	Bucket string `yaml:"bucket"`
	// Prefix is an optional key prefix to filter objects (e.g. "photos/").
	Prefix string `yaml:"prefix"`
	// Region is the AWS region (e.g. "us-east-1"). Required for AWS S3; for RustFS use any non-empty value.
	Region string `yaml:"region"`
	// AccessKey is the AWS access key ID or equivalent credential.
	// Leave empty for anonymous access to public buckets.
	AccessKey string `yaml:"accessKey"`
	// SecretKey is the AWS secret access key or equivalent credential.
	// Leave empty for anonymous access to public buckets.
	SecretKey string `yaml:"secretKey"`
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
// Returns an error if the required Blogs field is empty.
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
	if len(cfg.Blogs) == 0 {
		return nil, fmt.Errorf("tumblr scheduler config %s: blogs is required", path)
	}
	return &cfg, nil
}

// LoadS3Config reads and parses a YAML s3 scheduler config from the given path.
// Returns an error if any required field (Endpoint, Bucket, Region, AccessKey, SecretKey) is empty.
func LoadS3Config(path string) (*S3FileConfig, error) {
	data, err := readConfigFile(path)
	if err != nil {
		return nil, err
	}

	var cfg S3FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse s3 scheduler config %s: %w", path, err)
	}

	if err := applyDefaults(&cfg.SchedulerFileConfig); err != nil {
		return nil, err
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("s3 scheduler config %s: endpoint is required", path)
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 scheduler config %s: bucket is required", path)
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("s3 scheduler config %s: region is required", path)
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
	switch cfg.OnExternalImages {
	case "", "ignore", "takeover", "yield":
		// valid
	default:
		return fmt.Errorf("onExternalImages must be ignore, takeover, or yield (got %q)", cfg.OnExternalImages)
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
