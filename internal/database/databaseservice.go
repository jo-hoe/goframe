package database

import (
	"context"
	"fmt"
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
	GetCurrentImageID(ctx context.Context) (string, error)

	// GetCurrentImageURL returns the browser-facing URL for the given image ID and
	// variant ("original" or "processed"). The URL is routed through the ingress.
	GetCurrentImageURL(ctx context.Context, id, variant string) (string, error)

	// GetLastRotatedTime returns the timestamp of the last rotation advance.
	GetLastRotatedTime(ctx context.Context) (time.Time, error)

	// SetRotationKeys atomically writes the current image ID and last-rotated timestamp.
	SetRotationKeys(ctx context.Context, currentID string, rotatedAt time.Time) error
}

// NewDatabaseWithNamespace constructs a DatabaseService from the given config.
// dbType must be "rustfs". endpoint is the RustFS base URL, bucket is the S3
// bucket name (used as the namespace), accessKey/secretKey are the credentials,
// dbPath is the local SQLite file path, and imageBaseURL is the browser-facing
// URL prefix for image assets (e.g. "/images").
func NewDatabaseWithNamespace(dbType, endpoint, bucket, accessKey, secretKey, dbPath, imageBaseURL string) (DatabaseService, error) {
	switch dbType {
	case "rustfs":
		return NewRustFSDatabase(endpoint, bucket, accessKey, secretKey, "us-east-1", dbPath, imageBaseURL)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", dbType)
	}
}
