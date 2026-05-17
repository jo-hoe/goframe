package database

import (
	"fmt"
	"log/slog"
)

func NewDatabase(databaseType, connectionString string) (database DatabaseService, err error) {
	switch databaseType {
	case "sqlite":
		database, err = NewSQLiteDatabase(connectionString)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", databaseType)
	}

	slog.Info("initializing database schema", "driver", databaseType, "dsn", connectionString)
	err = database.CreateDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return database, nil
}
