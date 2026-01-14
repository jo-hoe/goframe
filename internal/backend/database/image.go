package database

type Image struct {
	ID             string `db:"id"`
	OriginalImage  []byte `db:"original_image"`  // PNG image data stored as binary
	ProcessedImage []byte `db:"processed_image"` // PNG image data stored as binary
	CreatedAt      string `db:"created_at"`      // ISO-8601 timestamp when the image row was created
}
