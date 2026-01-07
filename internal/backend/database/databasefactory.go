package database

import (
	"fmt"
	"log"
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

	// Ensure database schema exists (idempotent), important for in-memory SQLite
	log.Print("initializing database schema (ensuring tables exist)")
	if _, err = database.CreateDatabase(); err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return database, nil
}
