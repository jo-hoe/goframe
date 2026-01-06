package database

type Entry struct {
	ID             string
	OriginalImage  []byte // PNG image data stored as binary
	ProcessedImage []byte // PNG image data stored as binary
}
