package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jo-hoe/goframe/internal/backend"
	"github.com/jo-hoe/goframe/internal/core"
	frontend "github.com/jo-hoe/goframe/internal/frontend"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	coreService := core.NewCoreService(config)
	server := defineServer()

	apiService := backend.NewAPIService(config, coreService)
	apiService.SetRoutes(server)
	frontendService := frontend.NewFrontendService(config, coreService)
	frontendService.SetRoutes(server)

	portString := fmt.Sprintf(":%d", config.Port)

	server.Logger.Fatal(server.Start(portString))
}

func defineServer() *echo.Echo {
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Pre(middleware.RemoveTrailingSlash())

	e.Validator = &GenericEchoValidator{}

	return e
}
