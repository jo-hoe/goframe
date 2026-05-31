package database

import "time"

// Image holds per-image metadata. Blobs are stored in RustFS and accessed via URL redirects.
type Image struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}
