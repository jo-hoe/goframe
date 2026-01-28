package database

// nextRank returns a new rank string that sorts lexicographically after the given previous rank.
// This is a minimal, append-only LexoRank variant suitable for adding new items at the end of the list.
// For future needs (e.g., inserting between two ranks), implement a full LexoRank Between function.
func nextRank(prev string) string {
	const midChar = 'U'
	if prev == "" {
		return string([]rune{midChar})
	}
	return prev + string([]rune{midChar})
}
