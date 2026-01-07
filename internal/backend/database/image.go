package database

type Image struct {
	ID             string
	OriginalImage  []byte // PNG image data stored as binary
	ProcessedImage []byte // PNG image data stored as binary
}
