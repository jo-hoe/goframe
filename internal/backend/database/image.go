package database

type Image struct {
	ID             string `db:"id"`
	OriginalImage  []byte `db:"original_image"`  // PNG image data stored as binary
	ProcessedImage []byte `db:"processed_image"` // PNG image data stored as binary
	Rank           string `db:"rank"`            // LexoRank string to maintain ordering
}
