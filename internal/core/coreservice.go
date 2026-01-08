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

	latestImage := images[len(images)-1]
	slog.Info("CoreService.GetCurrentImage: using latest image", "id", latestImage.ID)

	processedImageData, err := service.databaseService.GetProcessedImageByID(latestImage.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get processed image by ID: %w", err)
	}

	slog.Info("CoreService.GetCurrentImage: returning processed image", "bytes", len(processedImageData))
	return processedImageData, nil
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
	slog.Info("CoreService.AddImage: image converted to PNG", "converted_bytes", len(convertedImageData))

	databaseImageID, err := service.databaseService.CreateImage(image)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}
	slog.Info("CoreService.AddImage: original image stored", "id", databaseImageID, "bytes", len(image))

	if err := service.databaseService.SetProcessedImage(databaseImageID, convertedImageData); err != nil {
		return nil, fmt.Errorf("failed to set processed image: %w", err)
	}
	slog.Info("CoreService.AddImage: processed image stored", "id", databaseImageID, "bytes", len(convertedImageData))

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
