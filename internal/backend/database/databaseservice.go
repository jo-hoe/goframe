package database

import "database/sql"

type DatabaseService interface {
	CreateDatabase() (*sql.DB, error)
	DoesDatabaseExist() bool
	Close() error

	CreateImage(image []byte) (*Image, error)
	AddProcessedImage(id string, processedImage []byte) error
	GetAllImages() ([]*Image, error)
	DeleteImage(id string) error
	GetImageByID(id string) (*Image, error)
}
