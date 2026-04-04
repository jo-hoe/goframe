package database

import (
	"regexp"
	"testing"
)

func Test_generateID_FormatAndUniqueness(t *testing.T) {
	// UUID v4 pattern: 8-4-4-4-12 hex, version 4 and variant 10xx
	uuidV4Pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	const n = 256
	seen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		got, err := generateID()
		if err != nil {
			t.Fatalf("generateID() returned error: %v", err)
		}
		if !uuidV4Pattern.MatchString(got) {
			t.Fatalf("generateID() returned invalid UUID v4 format: %q", got)
		}
		if _, dup := seen[got]; dup {
			t.Fatalf("generateID() returned duplicate UUID: %q", got)
		}
		seen[got] = struct{}{}
	}
}
