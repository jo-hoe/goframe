package database

import "database/sql"

type DatabaseService interface {
	CreateDatabase() (*sql.DB, error)
	DoesDatabaseExist() bool
	Close() error

	// CreateImage inserts a new image row with both original and processed image in a single transaction,
	// eliminating race conditions where processed_image is temporarily NULL.
	CreateImage(original []byte, processed []byte) (string, error)
	SetProcessedImage(id string, processedImage []byte) error
	GetAllImages() ([]*Image, error)
	DeleteImage(id string) error
	GetOriginalImageByID(id string) ([]byte, error)
	GetProcessedImageByID(id string) ([]byte, error)
}
