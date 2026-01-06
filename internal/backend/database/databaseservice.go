package database

import "database/sql"

type DatabaseService interface {
	CreateDatabase() (*sql.DB, error)
	DoesDatabaseExist() bool

	CreateEntry(image []byte) (*Entry, error)
	AddProccessedImage(id string, processedImage []byte) error
	GetAllEntrys() ([]*Entry, error)
	DeleteEntry(id string) error
	GetEntryByID(id string) (*Entry, error)
}
