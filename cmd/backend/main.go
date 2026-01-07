package main

import (
	"log"
	"os"
	"path/filepath"

	backend "github.com/jo-hoe/goframe/internal/backend"
	"github.com/jo-hoe/goframe/internal/core"
	frontend "github.com/jo-hoe/goframe/internal/frontend"
)

func getConfigPath() string {
	// First check if config path is provided via environment variable
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		return configPath
	}

	// Default to config/config.yaml in current working directory
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(cwd, "config", "config.yaml")
}

func main() {
	// Load configuration
	configPath := getConfigPath()
	config, err := core.LoadConfig(configPath)
	if err != nil {
		log.Printf("failed to load config from %s: %v", configPath, err)
		panic(err)
	}

	// Initialize CoreService
	coreService := core.NewCoreService(config)

	// Start the API apiService
	apiService := backend.NewAPIService(config.Port, config.ImageTargetType)
	apiService.Start()

	frontendService := frontend.NewFrontendService(coreService)
	_ = frontendService
}
