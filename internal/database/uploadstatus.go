package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
)

// UploadStatus is the derived processing state of a content-addressed upload.
// It is never stored directly; it is computed from which objects exist under
// the image's key prefix (see GetUploadState).
type UploadStatus string

const (
	// StatusUnknown means no markers and no processed blob exist for the ID.
	StatusUnknown UploadStatus = "unknown"
	// StatusProcessing means the processing marker is present.
	StatusProcessing UploadStatus = "processing"
	// StatusSucceeded means the processed blob exists.
	StatusSucceeded UploadStatus = "succeeded"
	// StatusFailed means a failure marker is present.
	StatusFailed UploadStatus = "failed"
)

// UploadState is the derived state returned to callers polling an upload.
type UploadState struct {
	ID     string       `json:"id"`
	Status UploadStatus `json:"status"`
	Error  string       `json:"error,omitempty"`
}

// contentIDPattern validates a content-addressed image ID (SHA-256 hex).
// IDs are used directly in object keys, so they must be strictly validated to
// prevent path traversal or key injection.
var contentIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ContentID returns the SHA-256 hex digest of the given bytes. This is the
// content-addressed image ID: identical bytes always yield the same ID, which
// makes uploads naturally idempotent.
func ContentID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ValidateContentID reports whether id is a well-formed content-addressed ID.
func ValidateContentID(id string) error {
	if !contentIDPattern.MatchString(id) {
		return fmt.Errorf("invalid image id %q: must be a 64-char lowercase hex sha256", id)
	}
	return nil
}

// imageProcessingMarkerKey returns the S3 key for the "processing" marker.
func imageProcessingMarkerKey(id string) string { return "images/" + id + "/status/processing" }

// imageFailedMarkerKey returns the S3 key for the "failed" marker.
func imageFailedMarkerKey(id string) string { return "images/" + id + "/status/failed.json" }

// failureMarker is the JSON payload stored in the failed marker object.
type failureMarker struct {
	Error string `json:"error"`
}

// deriveUploadState computes the upload state for id from the presence of the
// processed blob and the status markers, in precedence order. It performs at
// most three GETs and requires no object listing.
//
// getObject and objectExists are injected so the same logic backs both the
// RustFS implementation and the in-memory fake.
func deriveUploadState(
	ctx context.Context,
	id string,
	objectExists func(context.Context, string) (bool, error),
	getObject func(context.Context, string) ([]byte, error),
) (*UploadState, error) {
	if err := ValidateContentID(id); err != nil {
		return nil, err
	}

	processed, err := objectExists(ctx, imageProcessedKey(id))
	if err != nil {
		return nil, err
	}
	if processed {
		return &UploadState{ID: id, Status: StatusSucceeded}, nil
	}

	failed, err := getObject(ctx, imageFailedMarkerKey(id))
	if err != nil {
		return nil, err
	}
	if failed != nil {
		return &UploadState{ID: id, Status: StatusFailed, Error: parseFailureMarker(failed)}, nil
	}

	processing, err := objectExists(ctx, imageProcessingMarkerKey(id))
	if err != nil {
		return nil, err
	}
	if processing {
		return &UploadState{ID: id, Status: StatusProcessing}, nil
	}

	return &UploadState{ID: id, Status: StatusUnknown}, nil
}

// parseFailureMarker extracts the error message from a failed marker payload,
// tolerating malformed content.
func parseFailureMarker(data []byte) string {
	var m failureMarker
	if err := json.Unmarshal(data, &m); err != nil || m.Error == "" {
		return "image processing failed"
	}
	return m.Error
}

// newFailureMarker serialises a failure marker payload for the given error.
func newFailureMarker(errMsg string) ([]byte, error) {
	if errMsg == "" {
		errMsg = "image processing failed"
	}
	return json.Marshal(failureMarker{Error: errMsg})
}
