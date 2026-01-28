package database

import "database/sql"

type DatabaseService interface {
	CreateDatabase() (*sql.DB, error)
	Close() error

	// CreateImage inserts a new image row with both original and processed image in a single transaction,
	// eliminating race conditions where processed_image is temporarily NULL.
	CreateImage(original []byte, processed []byte) (string, error)
	// GetImages returns images with only the specified fields populated; if no fields are provided, all fields are returned.
	GetImages(fields ...string) ([]*Image, error)
	DeleteImage(id string) error
	GetImageByID(id string) (*Image, error)

	// UpdateRanks applies a new ordering to images by rewriting their LexoRank values in the given order atomically.
	// The first item gets the base rank, and each subsequent item gets nextRank of the previous.
	UpdateRanks(order []string) error
}
