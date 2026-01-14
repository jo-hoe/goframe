package database

import "time"

type Image struct {
	ID             string    `db:"id"`
	OriginalImage  []byte    `db:"original_image"`  // PNG image data stored as binary
	ProcessedImage []byte    `db:"processed_image"` // PNG image data stored as binary
	CreatedAt      time.Time `db:"created_at"`      // Timestamp when the image row was created
}
