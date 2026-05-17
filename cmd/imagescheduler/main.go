package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/imageprocessing"
	"github.com/jo-hoe/goframe/internal/scheduler"
	"github.com/jo-hoe/goframe/internal/scheduler/metmuseum"
	"github.com/jo-hoe/goframe/internal/scheduler/oatmeal"
	s3source "github.com/jo-hoe/goframe/internal/scheduler/s3"
	"github.com/jo-hoe/goframe/internal/scheduler/tumblr"
	"github.com/jo-hoe/goframe/internal/scheduler/xkcd"

	// Trigger command registrations.
	_ "github.com/jo-hoe/goframe/internal/imageprocessing"
)

func main() {
	path := configFilePath()

	// Peek at the source field before full config load so we know which typed config to use.
	sourceName, err := config.PeekSource(path)
	if err != nil {
		slog.Error("image-scheduler: failed to read source", "path", path, "error", err)
		os.Exit(1)
	}

	var (
		baseCfg *config.SchedulerFileConfig
		source  scheduler.ImageSource
	)

	switch strings.ToLower(sourceName) {
	case "metmuseum":
		mmCfg, loadErr := config.LoadMetMuseumConfig(path)
		if loadErr != nil {
			slog.Error("image-scheduler: failed to load config", "path", path, "error", loadErr)
			os.Exit(1)
		}
		baseCfg = &mmCfg.SchedulerFileConfig
		source = metmuseum.NewMetMuseumSource(mmCfg.DepartmentIDs)
	case "tumblr":
		tCfg, loadErr := config.LoadTumblrConfig(path)
		if loadErr != nil {
			slog.Error("image-scheduler: failed to load config", "path", path, "error", loadErr)
			os.Exit(1)
		}
		baseCfg = &tCfg.SchedulerFileConfig
		source = tumblr.NewTumblrSource(tCfg.Blogs)
	case "s3":
		s3Cfg, loadErr := config.LoadS3Config(path)
		if loadErr != nil {
			slog.Error("image-scheduler: failed to load config", "path", path, "error", loadErr)
			os.Exit(1)
		}
		baseCfg = &s3Cfg.SchedulerFileConfig
		source = s3source.NewS3Source(s3source.Config{
			Endpoint:  s3Cfg.Endpoint,
			Bucket:    s3Cfg.Bucket,
			Prefix:    s3Cfg.Prefix,
			Region:    s3Cfg.Region,
			AccessKey: fileOr(s3CredentialPath("accessKey"), s3Cfg.AccessKey),
			SecretKey: fileOr(s3CredentialPath("secretKey"), s3Cfg.SecretKey),
		})
	default:
		baseCfg, err = config.LoadSchedulerConfig(path)
		if err != nil {
			slog.Error("image-scheduler: failed to load config", "path", path, "error", err)
			os.Exit(1)
		}
		source = buildSource(baseCfg.Source)
	}

	if baseCfg.GoframeURL == "" {
		slog.Error("image-scheduler: goframeURL is required but not set", "path", path)
		os.Exit(1)
	}
	if baseCfg.SourceName == "" {
		slog.Error("image-scheduler: sourceName is required but not set", "path", path)
		os.Exit(1)
	}

	level := parseLogLevel(baseCfg.LogLevel)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	if source == nil {
		slog.Info("image-scheduler: no image source configured, nothing to do")
		return
	}

	cmdCfgs := make([]imageprocessing.CommandConfig, 0, len(baseCfg.Commands))
	for _, c := range baseCfg.Commands {
		cmdCfgs = append(cmdCfgs, imageprocessing.CommandConfig{Name: c.Name, Params: c.Params})
	}

	runCfg := scheduler.Config{
		GoframeBaseURL: baseCfg.GoframeURL,
		SourceName:     baseCfg.SourceName,
		KeepCount:      baseCfg.KeepCount,
		WhenUnmanaged:  scheduler.WhenUnmanaged(baseCfg.WhenUnmanaged),
		ExclusionGroup: baseCfg.ExclusionGroup,
		GroupMembers:   baseCfg.GroupMembers,
		Source:         source,
		Commands:       cmdCfgs,
	}

	if err := scheduler.RunOnce(context.Background(), runCfg); err != nil {
		slog.Error("image-scheduler: run failed", "error", err)
		os.Exit(1)
	}
}

func configFilePath() string {
	if p := os.Getenv("IMAGE_SCHEDULER_CONFIG_PATH"); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "local.yaml"
	}
	return filepath.Join(cwd, "local.yaml")
}

// buildSource returns the ImageSource for the given name, or nil if unrecognised.
func buildSource(name string) scheduler.ImageSource {
	switch strings.ToLower(name) {
	case "xkcd":
		return xkcd.NewXKCDSource()
	case "oatmeal":
		return oatmeal.NewOatmealSource()
	}
	return nil
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

const s3CredentialsMountPath = "/etc/s3-credentials" //nolint:gosec // mount path, not a credential

func s3CredentialPath(key string) string {
	return filepath.Join(s3CredentialsMountPath, key)
}

// fileOr reads a file and returns its trimmed content, or fallback if the file is absent or empty.
// strings.TrimSpace strips the trailing newline Kubernetes adds to mounted Secret files.
func fileOr(path, fallback string) string {
	data, err := os.ReadFile(path) //nolint:gosec // path is built from a known constant prefix
	if err != nil || len(data) == 0 {
		return fallback
	}
	return strings.TrimSpace(string(data))
}
