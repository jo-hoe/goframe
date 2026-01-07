package core

import (
	"fmt"
	"log/slog"

	"github.com/jo-hoe/goframe/internal/backend/database"
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

func (service *CoreService) addImage(image []byte) (*database.Image, error) {

	return service.databaseService.CreateImage(image)
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
