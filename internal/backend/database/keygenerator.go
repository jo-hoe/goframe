package database

import (
	"crypto/sha256"
	"fmt"
)

func generateID(data []byte) (string, error) {
	if data == nil {
		return "", fmt.Errorf("input data cannot be nil")
	}

	// Generate deterministic UUID v5 based on input data using SHA-256
	// Using a null namespace UUID (all zeros) for simplicity
	namespace := make([]byte, 16)

	// Create hash from namespace + data
	hash := sha256.New()
	hash.Write(namespace)
	hash.Write(data)
	sum := hash.Sum(nil)

	// Take first 16 bytes for UUID
	uuid := sum[:16]

	// Set version (5) and variant bits according to RFC 4122
	uuid[6] = (uuid[6] & 0x0f) | 0x50 // Version 5
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10

	// Format as UUID string: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16]), nil
}
