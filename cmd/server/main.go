package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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

	// Default to config.yaml in current working directory
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(cwd, "config.yaml")
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

	// Start HTTP server in a goroutine to allow graceful shutdown
	go func() {
		if err := server.Start(portString); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server error: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Printf("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	if err := coreService.Close(); err != nil {
		log.Printf("core service close error: %v", err)
	}
}

func defineServer() *echo.Echo {
	e := echo.New()

	// Configure request logger to skip "/" endpoint (health check/probe)
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/probe"
		},
		LogStatus:    true,
		LogLatency:   true,
		LogMethod:    true,
		LogURI:       true,
		LogError:     true,
		LogRemoteIP:  true,
		LogHost:      true,
		LogUserAgent: true,
		LogRoutePath: true,
		HandleError:  false,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				log.Printf("%s %s (route=%s) - Status: %d - Latency: %v - Error: %v - RemoteIP: %s - Host: %s - UA: %s",
					v.Method,
					v.URI,
					v.RoutePath,
					v.Status,
					v.Latency,
					v.Error,
					v.RemoteIP,
					v.Host,
					v.UserAgent,
				)
			} else {
				log.Printf("%s %s (route=%s) - Status: %d - Latency: %v - RemoteIP: %s - Host: %s - UA: %s",
					v.Method,
					v.URI,
					v.RoutePath,
					v.Status,
					v.Latency,
					v.RemoteIP,
					v.Host,
					v.UserAgent,
				)
			}
			return nil
		},
	}))

	e.Use(middleware.Recover())
	e.Pre(middleware.RemoveTrailingSlash())

	e.Validator = &GenericEchoValidator{}

	return e
}
