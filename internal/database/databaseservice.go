package database

import (
	"context"
	"fmt"
	"time"
)

type DatabaseService interface {
	Close() error

	// CreateImage uploads blobs to object storage and registers the image in the rotation state.
	// createdAt is stored as-is (caller is responsible for timezone).
	// source is an informational origin label (empty string for manual uploads).
	// afterID is the image ID to insert after in the display order; pass "" to append.
	CreateImage(ctx context.Context, original []byte, processed []byte, createdAt time.Time, source string, afterID string) (string, error)

	// GetImageMetadata returns all image metadata in current display order (index 0 = today).
	GetImageMetadata(ctx context.Context) ([]*Image, error)

	// GetImageByID returns metadata for a single image.
	GetImageByID(ctx context.Context, id string) (*Image, error)

	// DeleteImage removes an image from the rotation state and deletes its blobs.
	DeleteImage(ctx context.Context, id string) error

	// UpdateOrder replaces the display order with the given ID slice atomically.
	UpdateOrder(ctx context.Context, order []string) error

	// GetRotationOrderedIDs returns the full ordered ID list from rotation.json
	// (index 0 = today's image). This reflects the operator's latest rotation.
	GetRotationOrderedIDs(ctx context.Context) ([]string, error)

	// GetCurrentImageID returns the ID of the image currently selected for display.
	GetCurrentImageID(ctx context.Context) (string, error)

	// GetCurrentImageURL returns the browser-facing URL for the given image ID and
	// variant ("original" or "processed"). The URL is routed through the ingress.
	GetCurrentImageURL(ctx context.Context, id, variant string) (string, error)

	// GetLastRotatedTime returns the timestamp of the last rotation advance.
	GetLastRotatedTime(ctx context.Context) (time.Time, error)
}

// NewDatabaseWithNamespace constructs a DatabaseService from the given config.
// dbType must be "seaweedfs". endpoint is the S3 base URL, bucket is the S3
// bucket name (used as the namespace), accessKey/secretKey are the credentials,
// and imageBaseURL is the browser-facing URL prefix for image assets (e.g. "/images").
func NewDatabaseWithNamespace(dbType, endpoint, bucket, accessKey, secretKey, imageBaseURL string) (DatabaseService, error) {
	switch dbType {
	case "seaweedfs":
		return NewSeaweedFSDatabase(endpoint, bucket, accessKey, secretKey, "us-east-1", imageBaseURL)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", dbType)
	}
}
