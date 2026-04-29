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
	"github.com/jo-hoe/goframe/internal/scheduler/oatmeal"
	"github.com/jo-hoe/goframe/internal/scheduler/pusheen"
	"github.com/jo-hoe/goframe/internal/scheduler/xkcd"

	// Trigger command registrations.
	_ "github.com/jo-hoe/goframe/internal/imageprocessing"
)

func main() {
	path := configFilePath()
	fileCfg, err := config.LoadSchedulerConfig(path)
	if err != nil {
		log.Fatalf("image-scheduler: failed to load config from %s: %v", path, err)
	}
	if fileCfg.GoframeURL == "" {
		log.Fatalf("image-scheduler: goframeURL is required but not set in %s", path)
	}
	if fileCfg.SourceName == "" {
		log.Fatalf("image-scheduler: sourceName is required but not set in %s", path)
	}

	level := parseLogLevel(fileCfg.LogLevel)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	source := buildSource(fileCfg.Source)
	if source == nil {
		slog.Info("image-scheduler: no image source configured, nothing to do")
		return
	}

	cmdCfgs := make([]imageprocessing.CommandConfig, 0, len(fileCfg.Commands))
	for _, c := range fileCfg.Commands {
		cmdCfgs = append(cmdCfgs, imageprocessing.CommandConfig{Name: c.Name, Params: c.Params})
	}

	runCfg := scheduler.Config{
		GoframeBaseURL: fileCfg.GoframeURL,
		SourceName:     fileCfg.SourceName,
		KeepCount:      fileCfg.KeepCount,
		ExclusionGroup: fileCfg.ExclusionGroup,
		GroupMembers:   fileCfg.GroupMembers,
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
	case "pusheen":
		return pusheen.NewPusheenSource()
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
