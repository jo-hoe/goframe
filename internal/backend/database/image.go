package database

type Image struct {
	ID             string
	OriginalImage  []byte // PNG image data stored as binary
	ProcessedImage []byte // PNG image data stored as binary
	CreatedAt      string // ISO-8601 timestamp when the image row was created
}
