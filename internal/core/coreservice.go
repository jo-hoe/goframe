package core

import (
	"fmt"
	"log/slog"

	"github.com/jo-hoe/goframe/internal/backend/commands"
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

func (service *CoreService) GetCurrentImage() ([]byte, error) {
	slog.Info("CoreService.GetCurrentImage: fetching current image")

	images, err := service.databaseService.GetAllImages()
	if err != nil {
		return nil, fmt.Errorf("failed to get all images: %w", err)
	}
	slog.Info("CoreService.GetCurrentImage: images fetched", "count", len(images))

	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in database")
	}

	// Find the latest image that has a non-empty processed image
	for i := len(images) - 1; i >= 0; i-- {
		img := images[i]
		if len(img.ProcessedImage) > 0 {
			slog.Info("CoreService.GetCurrentImage: using latest processed image", "id", img.ID)
			return img.ProcessedImage, nil
		}
	}

	return nil, fmt.Errorf("no processed images available")
}

func (service *CoreService) AddImage(image []byte) (*common.ApiImage, error) {
	slog.Info("CoreService.AddImage: start", "bytes", len(image))

	command, err := commands.NewPngConverterCommand(map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}

	convertedImageData, err := command.Execute(image)
	if err != nil {
		return nil, fmt.Errorf("failed to convert image to PNG: %w", err)
	}

	// Insert atomically with processed image to avoid NULL windows
	databaseImageID, err := service.databaseService.CreateImage(image, convertedImageData)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}

	databaseImage := &common.ApiImage{
		ID: databaseImageID,
	}
	return databaseImage, nil
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
	slog.Info("CoreService.GetAllImageIDs: fetching image IDs")

	images, err := service.databaseService.GetAllImages()
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
	slog.Info("CoreService.GetProcessedImageByID: fetching processed image", "id", id)

	// First try to fetch the processed image directly
	if processed, err := service.databaseService.GetProcessedImageByID(id); err == nil && len(processed) > 0 {
		return processed, nil
	} else if err == nil && len(processed) == 0 {
		// Defensive: treat empty blob as missing
		slog.Warn("CoreService.GetProcessedImageByID: processed image empty, will attempt on-the-fly conversion", "id", id)
	} else if err != nil {
		slog.Warn("CoreService.GetProcessedImageByID: processed image not available, will attempt on-the-fly conversion", "id", id, "error", err)
	}

	// Fallback: fetch original and convert on the fly
	original, err := service.databaseService.GetOriginalImageByID(id)
	if err != nil || len(original) == 0 {
		if err != nil {
			return nil, fmt.Errorf("failed to get original image by ID: %w", err)
		}
		return nil, fmt.Errorf("original image empty for id %s", id)
	}

	command, err := commands.NewPngConverterCommand(map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to create PNG converter command: %w", err)
	}

	converted, err := command.Execute(original)
	if err != nil {
		return nil, fmt.Errorf("failed to convert original image to PNG: %w", err)
	}

	// Best-effort store converted image to eliminate future fallback work
	if setErr := service.databaseService.SetProcessedImage(id, converted); setErr != nil {
		slog.Warn("CoreService.GetProcessedImageByID: failed to persist converted processed image", "id", id, "error", setErr)
	}

	return converted, nil
}
