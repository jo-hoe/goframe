package database

import (
	"sort"
	"testing"
)

func TestNext(t *testing.T) {
	if got := Next(""); got != "U" {
		t.Fatalf("Next(\"\") = %q, want %q", got, "U")
	}
	if got := Next("U"); got != "UU" {
		t.Fatalf("Next(\"U\") = %q, want %q", got, "UU")
	}
}

func TestBetweenUnboundedUpper(t *testing.T) {
	if got := Between("U", ""); got != "UU" {
		t.Fatalf("Between(\"U\", \"\") = %q, want %q", got, "UU")
	}
}

func TestBetweenBoundedMidpoint(t *testing.T) {
	got := Between("A", "C")
	if !(got > "A" && got < "C") {
		t.Fatalf("Between(\"A\",\"C\") = %q, want strictly between A and C", got)
	}
}

func TestIsBetween(t *testing.T) {
	if !IsBetween("A", "B", "C") {
		t.Fatal("IsBetween(\"A\",\"B\",\"C\") = false, want true")
	}
	if IsBetween("A", "A", "C") {
		t.Fatal("IsBetween(\"A\",\"A\",\"C\") = true, want false (strictly between)")
	}
	if !IsBetween("", "A", "B") {
		t.Fatal("IsBetween(\"\", \"A\", \"B\") = false, want true")
	}
	if !IsBetween("A", "B", "") {
		t.Fatal("IsBetween(\"A\", \"B\", \"\") = false, want true")
	}
	if IsBetween("", "A", "") {
		t.Fatal("IsBetween(\"\", \"A\", \"\") = true, want false (no bounds)")
	}
}

func TestReorder_NoChange(t *testing.T) {
	existing := map[string]string{
		"a": "A",
		"b": "B",
		"c": "C",
	}
	order := []string{"a", "b", "c"}
	upd := Reorder(existing, order)
	if len(upd) != 0 {
		t.Fatalf("expected no updates, got: %+v", upd)
	}
}

func TestReorder_SwapAdjacent(t *testing.T) {
	existing := map[string]string{
		"a": "A",
		"b": "B",
		"c": "C",
	}
	order := []string{"b", "a", "c"}
	upd := Reorder(existing, order)

	// Minimal updates: expect at least one change, but not necessarily both neighbors.
	if len(upd) == 0 {
		t.Fatalf("expected at least one update, got none")
	}

	// Apply updates and verify final ordering b < a < c
	ranks := map[string]string{
		"a": existing["a"],
		"b": existing["b"],
		"c": existing["c"],
	}
	for id, r := range upd {
		ranks[id] = r
	}
	if !(ranks["b"] < ranks["a"] && ranks["a"] < ranks["c"]) {
		t.Fatalf("final ranks not ordered as b < a < c: %+v", ranks)
	}
}

func TestReorder_InsertAtFrontWithEmptyRank(t *testing.T) {
	// Simulate an item 'd' without rank yet that should go to the front.
	existing := map[string]string{
		"a": "B", // assume A < B < C in lexicographic space
		"b": "C",
		"c": "D",
		"d": "",
	}
	order := []string{"d", "a", "b", "c"}
	upd := Reorder(existing, order)
	if _, ok := upd["d"]; !ok {
		t.Fatalf("expected an update for 'd', got: %+v", upd)
	}
	// New rank for d must be less than 'a' rank
	newD := upd["d"]
	if !(newD < existing["a"]) {
		t.Fatalf("new rank for 'd' = %q, want less than %q", newD, existing["a"])
	}
	// Ensure resulting order is strictly increasing
	final := map[string]string{
		"a": existing["a"],
		"b": existing["b"],
		"c": existing["c"],
		"d": newD,
	}
	ids := []string{"d", "a", "b", "c"}
	for i := 0; i < len(ids)-1; i++ {
		if !(final[ids[i]] < final[ids[i+1]]) {
			t.Fatalf("final ranks not strictly increasing at %s < %s: %+v", ids[i], ids[i+1], final)
		}
	}
}

func TestReorder_MinimalUpdates(t *testing.T) {
	// Only the moved item should change ideally; neighbors should remain if already valid.
	existing := map[string]string{
		"a": "A",
		"b": "B",
		"c": "C",
		"d": "D",
	}
	order := []string{"a", "c", "b", "d"} // move 'c' before 'b'
	upd := Reorder(existing, order)
	// 'c' likely needs an update; 'a','b','d' often can remain
	if _, ok := upd["c"]; !ok {
		t.Fatalf("expected an update for 'c', got: %+v", upd)
	}
	// Verify ordering after applying updates
	final := map[string]string{}
	for id, r := range existing {
		final[id] = r
	}
	for id, r := range upd {
		final[id] = r
	}
	idx := map[string]int{"a": 0, "c": 1, "b": 2, "d": 3}
	ids := []string{"a", "c", "b", "d"}
	sort.SliceStable(ids, func(i, j int) bool {
		return final[ids[i]] < final[ids[j]]
	})
	for i, id := range ids {
		if idx[id] != i {
			t.Fatalf("final order does not match desired; got %v", ids)
		}
	}
}
