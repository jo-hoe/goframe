package database

import "database/sql"

type DatabaseService interface {
	CreateDatabase() (*sql.DB, error)
	DoesDatabaseExist() bool
	Close() error

	CreateImage(image []byte) (string, error)
	SetProcessedImage(id string, processedImage []byte) error
	GetAllImages() ([]*Image, error)
	DeleteImage(id string) error
	GetOriginalImageByID(id string) ([]byte, error)
	GetProcessedImageByID(id string) ([]byte, error)
}
