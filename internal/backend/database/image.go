package database

import "time"

type Image struct {
	ID             string    `db:"id"`
	CreatedAt      time.Time `db:"created_at"`    // ISO 8601 UTC timestamp of upload
	OriginalImage  []byte    `db:"original_image"` // PNG image data stored as binary
	ProcessedImage []byte    `db:"processed_image"` // PNG image data stored as binary
	Rank           string    `db:"rank"`            // LexoRank string to maintain ordering
}
