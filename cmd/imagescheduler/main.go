package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/imageprocessing"
	"github.com/jo-hoe/goframe/internal/scheduler"
	"github.com/jo-hoe/goframe/internal/scheduler/deviantart"
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
		log.Fatalf("image-scheduler: failed to read source from %s: %v", path, err)
	}

	var (
		baseCfg *config.SchedulerFileConfig
		source  scheduler.ImageSource
	)

	switch strings.ToLower(sourceName) {
	case "deviantart":
		daCfg, loadErr := config.LoadDeviantArtConfig(path)
		if loadErr != nil {
			log.Fatalf("image-scheduler: failed to load config from %s: %v", path, loadErr)
		}
		baseCfg = &daCfg.SchedulerFileConfig
		source = deviantart.NewDeviantArtSource(daCfg.Query)
	case "metmuseum":
		mmCfg, loadErr := config.LoadMetMuseumConfig(path)
		if loadErr != nil {
			log.Fatalf("image-scheduler: failed to load config from %s: %v", path, loadErr)
		}
		baseCfg = &mmCfg.SchedulerFileConfig
		source = metmuseum.NewMetMuseumSource(mmCfg.DepartmentIDs)
	case "tumblr":
		tCfg, loadErr := config.LoadTumblrConfig(path)
		if loadErr != nil {
			log.Fatalf("image-scheduler: failed to load config from %s: %v", path, loadErr)
		}
		baseCfg = &tCfg.SchedulerFileConfig
		source = tumblr.NewTumblrSource(tCfg.Blogs)
	case "s3":
		s3Cfg, loadErr := config.LoadS3Config(path)
		if loadErr != nil {
			log.Fatalf("image-scheduler: failed to load config from %s: %v", path, loadErr)
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
			log.Fatalf("image-scheduler: failed to load config from %s: %v", path, err)
		}
		source = buildSource(baseCfg.Source)
	}

	if baseCfg.GoframeURL == "" {
		log.Fatalf("image-scheduler: goframeURL is required but not set in %s", path)
	}
	if baseCfg.SourceName == "" {
		log.Fatalf("image-scheduler: sourceName is required but not set in %s", path)
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
		log.Fatalf("image-scheduler: run failed: %v", err)
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

const s3CredentialsMountPath = "/etc/s3-credentials"

func s3CredentialPath(key string) string {
	return filepath.Join(s3CredentialsMountPath, key)
}

// fileOr reads a file and returns its trimmed content, or fallback if the file is absent or empty.
// strings.TrimSpace strips the trailing newline Kubernetes adds to mounted Secret files.
func fileOr(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return fallback
	}
	return strings.TrimSpace(string(data))
}
