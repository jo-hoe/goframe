package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jo-hoe/goframe/internal/apihandler"
	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/core"
	frontend "github.com/jo-hoe/goframe/internal/frontend"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func getConfigPath() string {
	configFlag := flag.String("config", "", "path to config file")
	flag.Parse()
	if *configFlag != "" {
		return *configFlag
	}
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		return configPath
	}

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(cwd, "local.yaml")
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		fallthrough
	default:
		return slog.LevelInfo
	}
}

func main() {
	configPath := getConfigPath()
	config, err := config.LoadServerConfig(configPath)
	if err != nil {
		log.Printf("failed to load config from %s: %v", configPath, err)
		panic(err)
	}

	level := parseLogLevel(config.LogLevel)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
	slog.Info("logging initialized", "level", config.LogLevel)

	coreService, err := core.NewCoreService(config)
	if err != nil {
		log.Fatalf("failed to initialise core service: %v", err)
	}
	server := defineServer()

	api := apihandler.NewAPIService(coreService)
	api.SetRoutes(server)
	frontendService := frontend.NewFrontendService(config, coreService)
	frontendService.SetRoutes(server)

	portString := fmt.Sprintf(":%d", config.Port)

	go func() {
		if err := server.Start(portString); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server error: %v", err)
		}
	}()

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
