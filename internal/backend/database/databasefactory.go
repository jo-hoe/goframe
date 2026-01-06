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

	if !database.DoesDatabaseExist() {
		log.Print("database does not exist, creating new database")
		_, err = database.CreateDatabase()
		if err != nil {
			return nil, fmt.Errorf("failed to create database: %w", err)
		}
	}

	return database, nil
}
