package database

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validID(t *testing.T, seed string) string {
	t.Helper()
	return ContentID([]byte(seed))
}

func TestContentID_Deterministic(t *testing.T) {
	a := ContentID([]byte("hello"))
	b := ContentID([]byte("hello"))
	c := ContentID([]byte("world"))
	if a != b {
		t.Fatalf("ContentID not deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("ContentID collision for different inputs")
	}
	if err := ValidateContentID(a); err != nil {
		t.Fatalf("ContentID output failed validation: %v", err)
	}
}

func TestValidateContentID(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", ContentID([]byte("x")), false},
		{"empty", "", true},
		{"too short", "abc123", true},
		{"uppercase", strings.ToUpper(ContentID([]byte("x"))), true},
		{"path traversal", "../../etc/passwd", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateContentID(tc.id)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateContentID(%q) err=%v, wantErr=%v", tc.id, err, tc.wantErr)
			}
		})
	}
}

func TestFakeDatabase_DerivedStatusLifecycle(t *testing.T) {
	ctx := context.Background()
	db := NewFakeDatabase("/images")
	id := validID(t, "lifecycle")

	// unknown initially
	assertStatus(t, db, id, StatusUnknown)

	// processing after marker
	if err := db.MarkProcessing(ctx, id); err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}
	assertStatus(t, db, id, StatusProcessing)

	// succeeded once the processed blob exists (CreateImage), markers cleared
	if err := db.CreateImage(ctx, id, []byte("orig"), []byte("proc"), time.Now(), "", ""); err != nil {
		t.Fatalf("CreateImage: %v", err)
	}
	if err := db.ClearStatusMarkers(ctx, id); err != nil {
		t.Fatalf("ClearStatusMarkers: %v", err)
	}
	assertStatus(t, db, id, StatusSucceeded)

	exists, err := db.ImageExists(ctx, id)
	if err != nil || !exists {
		t.Fatalf("ImageExists = %v, %v; want true, nil", exists, err)
	}
}

func TestFakeDatabase_FailedStatus(t *testing.T) {
	ctx := context.Background()
	db := NewFakeDatabase("/images")
	id := validID(t, "failure")

	if err := db.MarkProcessing(ctx, id); err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}
	if err := db.MarkFailed(ctx, id, "boom"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	state, err := db.GetUploadState(ctx, id)
	if err != nil {
		t.Fatalf("GetUploadState: %v", err)
	}
	if state.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", state.Status)
	}
	if state.Error != "boom" {
		t.Fatalf("error = %q, want boom", state.Error)
	}
}

func TestFakeDatabase_SucceededBeatsFailed(t *testing.T) {
	// A successful rerun (processed blob present) must take precedence over a
	// stale failed marker.
	ctx := context.Background()
	db := NewFakeDatabase("/images")
	id := validID(t, "rerun")

	if err := db.MarkFailed(ctx, id, "old failure"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if err := db.CreateImage(ctx, id, []byte("orig"), []byte("proc"), time.Now(), "", ""); err != nil {
		t.Fatalf("CreateImage: %v", err)
	}
	assertStatus(t, db, id, StatusSucceeded)
}

func TestFakeDatabase_CreateImageUpsertNoDuplicate(t *testing.T) {
	ctx := context.Background()
	db := NewFakeDatabase("/images")
	id := validID(t, "dup")

	for i := range 3 {
		if err := db.CreateImage(ctx, id, []byte("orig"), []byte("proc"), time.Now(), "", ""); err != nil {
			t.Fatalf("CreateImage #%d: %v", i, err)
		}
	}
	ids, err := db.GetRotationOrderedIDs(ctx)
	if err != nil {
		t.Fatalf("GetRotationOrderedIDs: %v", err)
	}
	count := 0
	for _, got := range ids {
		if got == id {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("id appears %d times in rotation order, want 1", count)
	}
}

func TestFakeDatabase_CreateImageRejectsBadID(t *testing.T) {
	ctx := context.Background()
	db := NewFakeDatabase("/images")
	if err := db.CreateImage(ctx, "not-a-hash", []byte("o"), []byte("p"), time.Now(), "", ""); err == nil {
		t.Fatal("expected error for invalid id")
	}
}

func assertStatus(t *testing.T, db *FakeDatabase, id string, want UploadStatus) {
	t.Helper()
	state, err := db.GetUploadState(context.Background(), id)
	if err != nil {
		t.Fatalf("GetUploadState: %v", err)
	}
	if state.Status != want {
		t.Fatalf("status = %q, want %q", state.Status, want)
	}
}

func TestFakeDatabase_StaleProcessingMarkerIsUnknown(t *testing.T) {
	// A processing marker older than the TTL was orphaned by a crashed worker;
	// it must derive to unknown so the same bytes can be re-submitted.
	db := NewFakeDatabase("/images")
	id := validID(t, "stale")

	stale, err := json.Marshal(processingMarker{StartedAt: time.Now().Add(-2 * DefaultProcessingMarkerTTL)})
	if err != nil {
		t.Fatalf("marshal stale marker: %v", err)
	}
	db.objects[imageProcessingMarkerKey(id)] = stale

	assertStatus(t, db, id, StatusUnknown)
}

func TestFakeDatabase_FreshProcessingMarkerIsProcessing(t *testing.T) {
	db := NewFakeDatabase("/images")
	id := validID(t, "fresh")

	fresh, err := json.Marshal(processingMarker{StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("marshal fresh marker: %v", err)
	}
	db.objects[imageProcessingMarkerKey(id)] = fresh

	assertStatus(t, db, id, StatusProcessing)
}

func TestFakeDatabase_EmptyProcessingMarkerIsUnknown(t *testing.T) {
	// Legacy markers (written before the timestamp field existed) and any
	// unparseable payload carry no start time and are treated as stale.
	db := NewFakeDatabase("/images")

	empty := validID(t, "empty")
	db.objects[imageProcessingMarkerKey(empty)] = []byte{}
	assertStatus(t, db, empty, StatusUnknown)

	garbage := validID(t, "garbage")
	db.objects[imageProcessingMarkerKey(garbage)] = []byte("not json")
	assertStatus(t, db, garbage, StatusUnknown)
}
