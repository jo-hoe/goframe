package core

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jo-hoe/goframe/internal/backend/commands"
	"github.com/jo-hoe/goframe/internal/backend/commandstructure"
	"github.com/jo-hoe/goframe/internal/backend/database"
	"github.com/jo-hoe/goframe/internal/common"
)

type CoreService struct {
	config          *ServiceConfig
	databaseService database.DatabaseService
}

func NewCoreService(config *ServiceConfig) *CoreService {
	databaseService, err := getDatabaseService(config)
	if err != nil {
		panic(err)
	}
	return &CoreService{
		config:          config,
		databaseService: databaseService,
	}
}

func (service *CoreService) GetImageForDate(date time.Time) ([]byte, error) {
	// Stateless, time-driven LIFO selection
	id, err := service.selectImageForTime(date)
	if err != nil {
		return nil, err
	}
	// it is possible that the selected image has changed in the mean time (was deleted)
	// the case is not handled here; just return error if image not found
	processed, err := service.databaseService.GetProcessedImageByID(id)
	if err != nil || len(processed) == 0 {
		slog.Warn("CoreService.GetImageForDate: selected image was calculated but was unavailable", "id", id)
		return nil, fmt.Errorf("no processed images available")
	}
	return processed, nil
}

func (service *CoreService) AddImage(image []byte) (*common.ApiImage, error) {
	slog.Info("CoreService.AddImage: start", "bytes", len(image))

	command, err := commands.NewPngConverterCommand(map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}

	// default PNG conversion
	convertedImageData, err := command.Execute(image)
	if err != nil {
		return nil, fmt.Errorf("failed to convert image to PNG: %w", err)
	}
	// apply all configured commands
	processedImage, err := service.applyConfiguredCommands(convertedImageData)
	if err != nil {
		return nil, fmt.Errorf("failed to apply configured commands: %w", err)
	}

	// Insert atomically with processed image to avoid NULL windows
	databaseImageID, err := service.databaseService.CreateImage(image, processedImage)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}

	databaseImage := &common.ApiImage{
		ID: databaseImageID,
	}
	return databaseImage, nil
}

func (service *CoreService) applyConfiguredCommands(image []byte) (processedImage []byte, err error) {
	if image == nil {
		return nil, fmt.Errorf("input image is nil")
	}

	// If no commands are configured, return the original image unchanged
	if service == nil || service.config == nil || len(service.config.Commands) == 0 {
		slog.Debug("CoreService.applyConfiguredCommands: no commands configured, returning original image", "bytes", len(image))
		return image, nil
	}

	// Map core.CommandConfig to commandstructure.CommandConfig
	commandConfigs := make([]commandstructure.CommandConfig, 0, len(service.config.Commands))
	for _, cfg := range service.config.Commands {
		commandConfigs = append(commandConfigs, commandstructure.CommandConfig{
			Name:   cfg.Name,
			Params: cfg.Params,
		})
	}

	slog.Info("CoreService.applyConfiguredCommands: executing configured commands", "count", len(commandConfigs), "input_size_bytes", len(image))
	out, execErr := commandstructure.ExecuteCommands(image, commandConfigs)
	if execErr != nil {
		return nil, fmt.Errorf("failed to apply configured commands: %w", execErr)
	}
	return out, nil
}

func (service *CoreService) selectImageForTime(now time.Time) (string, error) {
	// Load configured timezone, fallback to UTC on error
	loc, err := time.LoadLocation(service.config.RotationTimezone)
	if err != nil || loc == nil {
		slog.Warn("invalid rotation timezone; defaulting to UTC", "tz", service.config.RotationTimezone, "err", err)
		loc = time.UTC
	}
	nowTZ := now.In(loc)
	anchor := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)
	if nowTZ.Before(anchor) {
		// Should never happen in practice; clamp to anchor
		nowTZ = anchor
	}
	// Day-based bucket index
	days := int(nowTZ.Sub(anchor).Hours() / 24.0)

	// Fetch ascending by created_at; SQLite GetImages orders by created_at ASC, rowid ASC
	images, err := service.databaseService.GetImages("id", "processed_image")
	if err != nil {
		return "", fmt.Errorf("failed to fetch images: %w", err)
	}
	// Filter eligible (processed_image non-empty)
	eligible := make([]database.Image, 0, len(images))
	for _, img := range images {
		if img != nil && len(img.ProcessedImage) > 0 {
			eligible = append(eligible, *img)
		}
	}
	n := len(eligible)
	if n == 0 {
		return "", fmt.Errorf("no eligible images")
	}

	// LIFO: newest first. Since eligible is ascending, pick from end.
	idx := days % n
	if idx < 0 {
		idx += n
	}
	indexFromEnd := n - 1 - idx
	return eligible[indexFromEnd].ID, nil
}

func getDatabaseService(DatabaseConfig *ServiceConfig) (database.DatabaseService, error) {
	databaseService, err := database.NewDatabase(DatabaseConfig.Database.Type, DatabaseConfig.Database.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	slog.Info("database initialized successfully", "type", DatabaseConfig.Database.Type)
	// Additional core service startup logic can be added here
	return databaseService, nil
}

func (service *CoreService) GetAllImageIDs() ([]string, error) {
	images, err := service.databaseService.GetImages()
	if err != nil {
		return nil, fmt.Errorf("failed to get all images: %w", err)
	}

	// Only include images that have a processed image available
	ids := make([]string, 0, len(images))
	for _, img := range images {
		if len(img.ProcessedImage) > 0 {
			ids = append(ids, img.ID)
		}
	}
	slog.Info("CoreService.GetAllImageIDs: fetched image IDs", "count", len(ids))
	return ids, nil
}

func (service *CoreService) DeleteImage(id string) error {
	slog.Info("CoreService.DeleteImage: deleting image", "id", id)
	return service.databaseService.DeleteImage(id)
}

// Close gracefully closes underlying resources (e.g., database connections)
func (service *CoreService) Close() error {
	slog.Info("CoreService.Close: closing resources")
	return service.databaseService.Close()
}

func (service *CoreService) GetProcessedImageByID(id string) ([]byte, error) {
	processed, err := service.databaseService.GetProcessedImageByID(id)
	if err != nil || len(processed) == 0 {
		return nil, fmt.Errorf("processed image not available for id %s", id)
	}
	return processed, nil
}
