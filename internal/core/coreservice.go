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

func (service *CoreService) GetImageForDate() ([]byte, error) {
	images, err := service.databaseService.GetImages()
	if err != nil {
		return nil, fmt.Errorf("failed to get all images: %w", err)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in database")
	}

	// Find the latest image that has a non-empty processed image
	for i := len(images) - 1; i >= 0; i-- {
		img := images[i]
		if len(img.ProcessedImage) > 0 {
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
