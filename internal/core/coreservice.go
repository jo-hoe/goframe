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
		slog.Error("failed to initialize database service", "error", err)
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
		slog.Error("CoreService.GetCurrentImage: GetAllImages failed", "error", err)
		return nil, fmt.Errorf("failed to get all images: %w", err)
	}
	slog.Info("CoreService.GetCurrentImage: images fetched", "count", len(images))

	if len(images) == 0 {
		slog.Warn("CoreService.GetCurrentImage: no images found")
		return nil, fmt.Errorf("no images found in database")
	}

	latestImage := images[len(images)-1]
	slog.Info("CoreService.GetCurrentImage: using latest image", "id", latestImage.ID)

	processedImageData, err := service.databaseService.GetProcessedImageByID(latestImage.ID)
	if err != nil {
		slog.Error("CoreService.GetCurrentImage: GetProcessedImageByID failed", "id", latestImage.ID, "error", err)
		return nil, fmt.Errorf("failed to get processed image by ID: %w", err)
	}

	slog.Info("CoreService.GetCurrentImage: returning processed image", "bytes", len(processedImageData))
	return processedImageData, nil
}

func (service *CoreService) AddImage(image []byte) (*common.ApiImage, error) {
	targetType := service.config.ImageTargetType
	slog.Info("CoreService.AddImage: start", "bytes", len(image), "targetType", targetType)

	command, err := commands.NewImageConverterCommand(map[string]any{
		"targetType": targetType,
	})
	if err != nil {
		slog.Error("CoreService.AddImage: failed to create ImageConverterCommand", "error", err)
		return nil, fmt.Errorf("failed to create image converter command: %w", err)
	}

	convertedImageData, err := command.Execute(image)
	if err != nil {
		slog.Error("CoreService.AddImage: image conversion failed", "error", err)
		return nil, fmt.Errorf("failed to convert image: %w", err)
	}
	slog.Info("CoreService.AddImage: image converted", "converted_bytes", len(convertedImageData), "targetType", targetType)

	databaseImageID, err := service.databaseService.CreateImage(image)
	if err != nil {
		slog.Error("CoreService.AddImage: failed to create database image", "error", err)
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}
	slog.Info("CoreService.AddImage: original image stored", "id", databaseImageID, "bytes", len(image))

	if err := service.databaseService.SetProcessedImage(databaseImageID, convertedImageData); err != nil {
		slog.Error("CoreService.AddImage: failed to set processed image", "id", databaseImageID, "error", err)
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
