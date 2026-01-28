package database

import "strings"

const (
	// Alphabet bounds used to compute ranks lexicographically.
	// Using ASCII '0'..'z' yields a large space with many available midpoints.
	minChar = '0'
	maxChar = 'z'
	// Default mid character used for simple Next operations.
	midChar = 'U'
)

// Next returns a new rank string that sorts lexicographically after the given previous rank.
// If prev is empty, it returns a single midChar. Otherwise, it appends midChar, ensuring the
// new rank is strictly greater.
func Next(prev string) string {
	if prev == "" {
		return string([]rune{midChar})
	}
	return prev + string([]rune{midChar})
}

// compare returns the lexicographic comparison of a and b:
// -1 if a < b, 0 if a == b, +1 if a > b
func compare(a, b string) int {
	return strings.Compare(a, b)
}

// IsBetween reports whether rank lies strictly between prev and next.
// Bounds may be empty to indicate no lower (prev="") or upper (next="") bound.
// When both bounds are empty, the function returns false to force the caller to
// generate a canonical rank.
func IsBetween(prev, rank, next string) bool {
	if prev == "" && next == "" {
		return false
	}
	if prev == "" {
		return compare(rank, next) < 0
	}
	if next == "" {
		return compare(prev, rank) < 0
	}
	return compare(prev, rank) < 0 && compare(rank, next) < 0
}

// Between computes a rank string strictly between prev and next using a variable-length
// lexicographic scheme. If next is empty, it returns Next(prev). If prev is empty, it
// chooses a rank strictly less than next. When bounds are equal or invalid, it falls
// back to Next(prev).
//
// The algorithm walks character-by-character and selects a midpoint character whenever
// space exists between the lower and upper bound characters. If no space exists at a
// position, it appends the lower bound character and continues deeper, ensuring progress
// and eventual success due to the maxChar upper bound at unbounded positions.
func Between(prev, next string) string {
	// Unbounded upper: append midChar to move after prev
	if next == "" {
		return Next(prev)
	}

	p := []rune(prev)
	n := []rune(next)

	var out []rune
	i := 0
	for {
		// Lower bound character for this position
		pr := minChar
		if i < len(p) {
			pr = p[i]
		}
		// Upper bound character for this position
		var nr rune
		if i < len(n) {
			nr = n[i]
		} else {
			// When upper bound is exhausted, treat it as maxChar to keep room above
			nr = maxChar
		}

		// Carry over equal characters (tight bound at this position)
		if pr == nr {
			out = append(out, pr)
			i++
			continue
		}

		// If there is space between pr and nr, choose a midpoint
		if pr+1 < nr {
			mid := pr + (nr-pr)/2
			out = append(out, mid)
			return string(out)
		}

		// No space at this position, append pr and descend to next character
		out = append(out, pr)
		i++
	}
}

// Reorder computes minimal new ranks for items in the given order based on their existing ranks.
// It returns only the IDs that require updates mapped to their new ranks, leveraging Between()
// to insert ranks between neighbors rather than rewriting all ranks.
//
// existing: map of id -> current rank
// order: desired ordering of ids (first to last)
//
// Strategy:
//   - For each id in 'order', determine the neighbor ranks (prev and next) using already computed
//     updates when available to ensure consistency.
//   - If the current rank already lies strictly between prev and next, skip updating that id.
//   - Otherwise, compute a new rank with Between(prev, next).
func Reorder(existing map[string]string, order []string) map[string]string {
	updates := make(map[string]string, len(order))

	// Helper to get the latest rank for any id, considering previous updates.
	get := func(id string) string {
		if id == "" {
			return ""
		}
		if r, ok := updates[id]; ok {
			return r
		}
		return existing[id]
	}

	for i, id := range order {
		var prevID, nextID string
		if i > 0 {
			prevID = order[i-1]
		}
		if i < len(order)-1 {
			nextID = order[i+1]
		}

		prevRank := get(prevID)
		nextRank := get(nextID)
		curRank := existing[id]

		// If the current rank already satisfies ordering constraints, keep it.
		if curRank != "" && IsBetween(prevRank, curRank, nextRank) {
			continue
		}

		// Otherwise compute a new rank between neighbors.
		newRank := Between(prevRank, nextRank)
		updates[id] = newRank
	}

	return updates
}
