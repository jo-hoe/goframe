package scheduler

import "context"

// ImageSource fetches a single raw image for upload to goframe.
type ImageSource interface {
	// Name returns the source identifier used in logs.
	Name() string
	// Fetch retrieves raw image bytes from the source.
	Fetch(ctx context.Context) ([]byte, error)
}
