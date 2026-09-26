// Package imagevalidation provides fast, in-memory sanity checks for uploaded
// image bytes. It deliberately does no heavy decoding or processing: the goal
// is to reject obviously invalid uploads synchronously (before an upload is
// accepted for background processing), while leaving the full processing
// pipeline to run asynchronously.
package imagevalidation

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"unicode/utf8"

	// Register the raster decoders the pipeline supports so DecodeConfig can
	// recognise them by content.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// ErrEmpty is returned when the uploaded data is empty.
var ErrEmpty = errors.New("uploaded image is empty")

// ValidationError describes why an upload was rejected. It is a typed error so
// callers can map it to a 400 response without string matching.
type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string { return e.Reason }

// AsValidationError reports whether err is (or wraps) a *ValidationError.
func AsValidationError(err error) (*ValidationError, bool) {
	return errors.AsType[*ValidationError](err)
}

func newValidationError(format string, args ...any) *ValidationError {
	return &ValidationError{Reason: fmt.Sprintf(format, args...)}
}

// Validate performs lightweight validation on the uploaded image bytes.
//
// maxBytes, when > 0, is the maximum accepted size. It returns a *ValidationError
// for any user-correctable problem (empty, too large, unrecognised format).
func Validate(data []byte, maxBytes int64) error {
	if len(data) == 0 {
		return &ValidationError{Reason: ErrEmpty.Error()}
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return newValidationError("uploaded image is too large: %d bytes exceeds limit of %d", len(data), maxBytes)
	}

	// SVG is vector and not handled by image.DecodeConfig; accept it here as a
	// candidate and let the background pipeline do the full parse.
	if looksLikeSVG(data) {
		return nil
	}

	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return newValidationError("uploaded data is not a recognised image format: %v", err)
	} else if format == "" {
		return newValidationError("uploaded data is not a recognised image format")
	}
	return nil
}

// looksLikeSVG heuristically detects SVG content by scanning the leading bytes
// for an "<svg" tag, tolerating a leading XML declaration and whitespace.
func looksLikeSVG(data []byte) bool {
	const scan = 1024
	head := data
	if len(head) > scan {
		head = head[:scan]
	}
	if !utf8.Valid(head) {
		return false
	}
	return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}
