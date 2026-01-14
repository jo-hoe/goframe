package core

import (
	"testing"
	"time"
)

func newTestCoreService(t *testing.T, tz string) *CoreService {
	t.Helper()
	cfg := &ServiceConfig{
		Port: 0,
		Database: Database{
			Type:             "sqlite",
			ConnectionString: ":memory:",
		},
		RotationTimezone: tz,
	}
	svc := NewCoreService(cfg)
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// helper to get the timezone Location, defaulting to UTC if invalid
func mustLocation(t *testing.T, tz string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

func TestLIFOSelectionCycles(t *testing.T) {
	svc := newTestCoreService(t, "UTC")

	// Insert three eligible images (processed non-empty); insertion order defines created_at ASC position
	id1, err := svc.databaseService.CreateImage([]byte("orig1"), []byte("proc1"))
	if err != nil {
		t.Fatalf("CreateImage #1 error: %v", err)
	}
	id2, err := svc.databaseService.CreateImage([]byte("orig2"), []byte("proc2"))
	if err != nil {
		t.Fatalf("CreateImage #2 error: %v", err)
	}
	id3, err := svc.databaseService.CreateImage([]byte("orig3"), []byte("proc3"))
	if err != nil {
		t.Fatalf("CreateImage #3 error: %v", err)
	}

	loc := mustLocation(t, "UTC")
	anchor := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)

	// Expected LIFO sequence for consecutive days with 3 items: newest-first wrapping
	expected := []string{id3, id2, id1, id3, id2, id1}
	for k := 0; k < len(expected); k++ {
		now := anchor.Add(time.Hour * 24 * time.Duration(k))
		got, err := svc.selectImageForTime(now)
		if err != nil {
			t.Fatalf("selectImageIDLIFOForTime error at day %d: %v", k, err)
		}
		if got != expected[k] {
			t.Fatalf("day %d: expected %s, got %s", k, expected[k], got)
		}
	}
}

func TestDeletionMidDayAdvancesSelection(t *testing.T) {
	svc := newTestCoreService(t, "UTC")

	id1, err := svc.databaseService.CreateImage([]byte("orig1"), []byte("proc1"))
	if err != nil {
		t.Fatalf("CreateImage #1 error: %v", err)
	}
	id2, err := svc.databaseService.CreateImage([]byte("orig2"), []byte("proc2"))
	if err != nil {
		t.Fatalf("CreateImage #2 error: %v", err)
	}
	id3, err := svc.databaseService.CreateImage([]byte("orig3"), []byte("proc3"))
	if err != nil {
		t.Fatalf("CreateImage #3 error: %v", err)
	}

	loc := mustLocation(t, "UTC")
	anchor := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)

	// Day 0 should pick newest (id3)
	now := anchor
	chosen, err := svc.selectImageForTime(now)
	if err != nil {
		t.Fatalf("initial selection error: %v", err)
	}
	if chosen != id3 {
		t.Fatalf("expected newest id %s, got %s", id3, chosen)
	}

	// Delete the chosen image; selection should move to next LIFO (id2) for the same timestamp
	if err := svc.databaseService.DeleteImage(id3); err != nil {
		t.Fatalf("DeleteImage error: %v", err)
	}
	chosen2, err := svc.selectImageForTime(now)
	if err != nil {
		t.Fatalf("selection after deletion error: %v", err)
	}
	if chosen2 != id2 {
		t.Fatalf("after deletion expected next id %s, got %s (id1=%s)", id2, chosen2, id1)
	}
}

func TestTimezoneConfigured(t *testing.T) {
	// Use a non-UTC timezone and ensure selection still works
	svc := newTestCoreService(t, "Europe/Berlin")

	_, err := svc.databaseService.CreateImage([]byte("orig1"), []byte("proc1"))
	if err != nil {
		t.Fatalf("CreateImage #1 error: %v", err)
	}
	_, err = svc.databaseService.CreateImage([]byte("orig2"), []byte("proc2"))
	if err != nil {
		t.Fatalf("CreateImage #2 error: %v", err)
	}

	// Use a UTC time; service will convert internally to Europe/Berlin
	now := time.Date(2024, 1, 1, 22, 0, 0, 0, time.UTC) // near midnight in Berlin (winter UTC+1)
	_, err = svc.selectImageForTime(now)
	if err != nil {
		t.Fatalf("selection with timezone configured error: %v", err)
	}
}

func TestNoEligibleImages(t *testing.T) {
	svc := newTestCoreService(t, "UTC")

	// Insert an ineligible image (processed is nil)
	_, err := svc.databaseService.CreateImage([]byte("orig1"), nil)
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}

	now := time.Now()
	_, err = svc.selectImageForTime(now)
	if err == nil {
		t.Fatalf("expected error for no eligible images, got nil")
	}
}
