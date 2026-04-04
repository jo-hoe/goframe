package database

import (
	"context"
	"time"
)

type DatabaseService interface {
	Close() error

	// CreateImage inserts a new image with both original and processed blobs atomically.
	// createdAt is stored as-is (caller is responsible for timezone).
	// source is an informational origin label (empty string for manual uploads).
	// afterID is the image ID to insert after in the display queue; pass "" to append to end.
	CreateImage(ctx context.Context, original []byte, processed []byte, createdAt time.Time, source string, afterID string) (string, error)
	// GetImages returns images with only the specified fields populated; if no fields are provided, all fields are returned.
	GetImages(ctx context.Context, fields ...string) ([]*Image, error)
	// GetImagesBySource returns all images with the given source label, with id and rank populated, ordered by rank ASC.
	GetImagesBySource(ctx context.Context, source string) ([]*Image, error)
	DeleteImage(ctx context.Context, id string) error
	GetImageByID(ctx context.Context, id string) (*Image, error)

	// UpdateRanks resets image ordering to match the given ID slice atomically.
	UpdateRanks(ctx context.Context, order []string) error

	// GetCurrentImageID returns the ID of the image currently selected for display.
	// Redis: reads the operator-managed rotation:current-id key (falls back to first image if unset).
	GetCurrentImageID(ctx context.Context) (string, error)
}
