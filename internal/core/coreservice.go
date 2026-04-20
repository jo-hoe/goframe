package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jo-hoe/goframe/internal/common"
	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/database"
	"github.com/jo-hoe/goframe/internal/imageprocessing"

	// Import imageprocessing to trigger init() registrations for all commands.
	_ "github.com/jo-hoe/goframe/internal/imageprocessing"
)

// CoreService is the central business logic layer for the goframe server.
type CoreService struct {
	config          *config.ServiceConfig
	databaseService database.DatabaseService
	commandConfigs  []imageprocessing.CommandConfig
	tzLoc           *time.Location
}

// NewCoreService constructs and initialises a CoreService from the given config.
func NewCoreService(cfg *config.ServiceConfig) (*CoreService, error) {
	db, err := database.NewDatabaseWithNamespace(cfg.Database.Type, cfg.Database.ConnectionString, cfg.Database.Namespace)
	if err != nil {
		return nil, fmt.Errorf("initialising database: %w", err)
	}

	cmdCfgs := make([]imageprocessing.CommandConfig, 0, len(cfg.Commands))
	for _, c := range cfg.Commands {
		cmdCfgs = append(cmdCfgs, imageprocessing.CommandConfig{
			Name:   c.Name,
			Params: c.Params,
		})
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil || loc == nil {
		slog.Warn("invalid timezone; defaulting to UTC", "tz", cfg.Timezone, "err", err)
		loc = time.UTC
	}

	return &CoreService{
		config:          cfg,
		databaseService: db,
		commandConfigs:  cmdCfgs,
		tzLoc:           loc,
	}, nil
}

// AddImage processes and persists a new image.
func (service *CoreService) AddImage(ctx context.Context, image []byte, source string) (*common.ApiImage, error) {
	slog.Info("CoreService.AddImage: start", "bytes", len(image), "source", source)

	convertedImageData, processedImage, err := service.applyPipeline(image)
	if err != nil {
		return nil, err
	}

	// Determine where to insert: immediately after the current image of the day.
	// Falls back to "" (append to end) if no current image is set yet.
	afterID, err := service.databaseService.GetCurrentImageID(ctx)
	if err != nil {
		afterID = ""
	}

	databaseImageID, err := service.databaseService.CreateImage(ctx, convertedImageData, processedImage, time.Now().In(service.tzLoc), source, afterID)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}

	return &common.ApiImage{ID: databaseImageID}, nil
}

// GetImageById returns a single image by its ID.
func (service *CoreService) GetImageById(ctx context.Context, id string) (*database.Image, error) {
	return service.databaseService.GetImageByID(ctx, id)
}

// DeleteImage removes an image by its ID.
func (service *CoreService) DeleteImage(ctx context.Context, id string) error {
	slog.Info("CoreService.DeleteImage: deleting image", "id", id)
	return service.databaseService.DeleteImage(ctx, id)
}

// Close gracefully closes underlying resources.
func (service *CoreService) Close() error {
	slog.Info("CoreService.Close: closing resources")
	return service.databaseService.Close()
}

// GetOrderedImageIDs returns the persisted order of image IDs.
func (service *CoreService) GetOrderedImageIDs(ctx context.Context) ([]string, error) {
	return service.getOrderedImageIDs(ctx)
}

// GetOrderedImages returns images with id, created_at, and source populated, in rank order.
func (service *CoreService) GetOrderedImages(ctx context.Context) ([]*database.Image, error) {
	return service.databaseService.GetImages(ctx, "id", "created_at", "source")
}

// GetImageForTime returns the current image ID from the operator-managed rotation key.
// The time argument is unused; rotation is driven by the operator writing to Redis.
func (service *CoreService) GetImageForTime(ctx context.Context, _ time.Time) (string, error) {
	return service.databaseService.GetCurrentImageID(ctx)
}

// UpdateImageOrder updates the persistent order to match the given list of IDs.
func (service *CoreService) UpdateImageOrder(ctx context.Context, order []string) error {
	if len(order) == 0 {
		return nil
	}
	return service.databaseService.UpdateRanks(ctx, order)
}

func (service *CoreService) getOrderedImageIDs(ctx context.Context) ([]string, error) {
	images, err := service.databaseService.GetImages(ctx, "id")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch images: %w", err)
	}
	ids := make([]string, 0, len(images))
	for _, img := range images {
		ids = append(ids, img.ID)
	}
	return ids, nil
}

// applyPipeline converts the input image to PNG and applies the configured command pipeline.
// If NormalizeOrientationCommand is present in the pipeline, it runs first on the raw input
// bytes (before PNG conversion) so that EXIF orientation data is still available.
func (service *CoreService) applyPipeline(image []byte) (converted []byte, processed []byte, err error) {
	if image == nil {
		return nil, nil, fmt.Errorf("input image is nil")
	}

	// Run NormalizeOrientationCommand on raw bytes before PNG conversion if configured.
	preProcessed, remainingConfigs, err := applyPrePNGCommands(image, service.commandConfigs)
	if err != nil {
		return nil, nil, err
	}

	params := map[string]any{}
	if service.config.SvgFallbackLongSidePixelCount > 0 {
		params["svgFallbackLongSidePixelCount"] = service.config.SvgFallbackLongSidePixelCount
	}
	pngCmd, err := imageprocessing.NewPngConverterCommand(params)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}
	convertedImageData, err := pngCmd.Execute(preProcessed)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert image to PNG: %w", err)
	}

	if len(remainingConfigs) == 0 {
		slog.Debug("CoreService.applyPipeline: no commands configured, returning converted image", "bytes", len(convertedImageData))
		return convertedImageData, convertedImageData, nil
	}

	slog.Info("CoreService.applyPipeline: executing configured commands", "count", len(remainingConfigs), "input_size_bytes", len(convertedImageData))
	out, execErr := imageprocessing.ExecuteCommands(convertedImageData, remainingConfigs)
	if execErr != nil {
		return nil, nil, fmt.Errorf("failed to apply configured commands: %w", execErr)
	}
	return convertedImageData, out, nil
}

// applyPrePNGCommands runs any NormalizeOrientationCommand entries on raw image bytes
// before PNG conversion, so they receive the original format (e.g. JPEG) with EXIF
// metadata still intact. Non-JPEG formats (PNG, SVG, BMP, TIFF, WebP, GIF) are passed
// through unchanged. All other commands are deferred to the post-conversion pipeline.
// Returns the processed bytes and the remaining command configs.
func applyPrePNGCommands(image []byte, configs []imageprocessing.CommandConfig) ([]byte, []imageprocessing.CommandConfig, error) {
	remaining := make([]imageprocessing.CommandConfig, 0, len(configs))
	current := image
	for _, cfg := range configs {
		if cfg.Name != "NormalizeOrientationCommand" {
			remaining = append(remaining, cfg)
			continue
		}
		cmd, err := imageprocessing.DefaultRegistry.Create(cfg.Name, cfg.Params)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create %s: %w", cfg.Name, err)
		}
		current, err = cmd.Execute(current)
		if err != nil {
			return nil, nil, fmt.Errorf("%s failed: %w", cfg.Name, err)
		}
		slog.Info("CoreService.applyPipeline: applied pre-PNG command", "command", cfg.Name)
	}
	return current, remaining, nil
}
