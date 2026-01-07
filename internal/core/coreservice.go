package core

import (
	"fmt"
	"log/slog"

	"github.com/jo-hoe/goframe/internal/backend/command"
	"github.com/jo-hoe/goframe/internal/backend/database"
	"github.com/jo-hoe/goframe/internal/common"
)

type CoreService struct {
	config          *ServiceConfig
	databaseService database.DatabaseService
}

func (service *CoreService) GetConfig() ServiceConfig {
	return *service.config
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
	images, err := service.databaseService.GetAllImages()
	if err != nil {
		return nil, fmt.Errorf("failed to get all images: %w", err)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in database")
	}

	latestImage := images[len(images)-1]
	processedImageData, err := service.databaseService.GetProcessedImageByID(latestImage.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get processed image by ID: %w", err)
	}

	return processedImageData, nil
}

func (service *CoreService) AddImage(image []byte) (*common.ApiImage, error) {
	targetType := service.config.ImageTargetType
	command, err := command.NewImageConverterCommand(map[string]any{
		"targetType": targetType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create image converter command: %w", err)
	}

	convertedImageData, err := command.Execute(image)
	if err != nil {
		return nil, fmt.Errorf("failed to convert image: %w", err)
	}

	databaseImageID, err := service.databaseService.CreateImage(convertedImageData)
	if err != nil {
		return nil, fmt.Errorf("failed to create database image: %w", err)
	}
	service.databaseService.SetProcessedImage(databaseImageID, image)

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
