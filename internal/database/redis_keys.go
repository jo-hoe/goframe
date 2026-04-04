package database

import "fmt"

// Redis key constructors. All Redis key construction goes through these functions.
// Never use raw string format calls at Redis key sites.
//
// The namespace parameter corresponds to the GoFrame CR metadata.name,
// ensuring multiple CR instances in one cluster have isolated key spaces.

// ImageHashKey returns the hash key for a single image's metadata and blob fields.
func ImageHashKey(namespace, id string) string {
	return fmt.Sprintf("goframe:%s:image:%s", namespace, id)
}

// OrderSetKey returns the sorted-set key for the image ordering within a namespace.
// Scores are float64; normalised back to integers 1..N by UpdateRanks on every reorder.
func OrderSetKey(namespace string) string {
	return fmt.Sprintf("goframe:%s:images:order", namespace)
}

// RotationCurrentIDKey returns the string key holding the active image ID.
// Written by the operator at timezone-aware midnight.
func RotationCurrentIDKey(namespace string) string {
	return fmt.Sprintf("goframe:%s:rotation:current-id", namespace)
}

// RotationLastRotatedKey returns the string key holding the last rotation timestamp (RFC3339).
// Written by the operator for status reporting.
func RotationLastRotatedKey(namespace string) string {
	return fmt.Sprintf("goframe:%s:rotation:last-rotated", namespace)
}
