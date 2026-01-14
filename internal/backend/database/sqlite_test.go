package database

import (
	"bytes"
	"database/sql"
	"testing"
)

func newTestDB(t *testing.T) DatabaseService {
	t.Helper()

	ds, err := NewSQLiteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteDatabase error: %v", err)
	}
	_, err = ds.CreateDatabase()
	if err != nil {
		t.Fatalf("CreateDatabase error: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func TestSQLite_DoesDatabaseExist(t *testing.T) {
	ds := newTestDB(t)
	if !ds.DoesDatabaseExist() {
		t.Fatalf("expected DoesDatabaseExist to return true")
	}
}

func TestSQLite_GetImages_Projection(t *testing.T) {
	ds := newTestDB(t)

	id1, err := ds.CreateImage([]byte{0x01, 0x02}, []byte{0x10})
	if err != nil {
		t.Fatalf("CreateImage #1 error: %v", err)
	}
	id2, err := ds.CreateImage([]byte{0x03}, []byte{0x20})
	if err != nil {
		t.Fatalf("CreateImage #2 error: %v", err)
	}

	// Request only ID field
	images, err := ds.GetImages("id")
	if err != nil {
		t.Fatalf("GetImages(id) error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	seen := map[string]bool{}
	for i, img := range images {
		if img.ID == "" {
			t.Errorf("image[%d].ID is empty; expected non-empty", i)
		}
		if img.OriginalImage != nil {
			t.Errorf("image[%d].OriginalImage is not nil; expected nil when not selected", i)
		}
		if img.ProcessedImage != nil {
			t.Errorf("image[%d].ProcessedImage is not nil; expected nil when not selected", i)
		}
		if img.CreatedAt != "" {
			t.Errorf("image[%d].CreatedAt is not empty; expected empty when not selected", i)
		}
		seen[img.ID] = true
	}
	if !seen[id1] || !seen[id2] {
		t.Fatalf("expected IDs %q and %q to be present in results, got %v", id1, id2, seen)
	}

	// Request ID and created_at
	images2, err := ds.GetImages("id", "created_at")
	if err != nil {
		t.Fatalf("GetImages(id, created_at) error: %v", err)
	}
	if len(images2) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images2))
	}
	for i, img := range images2 {
		if img.ID == "" {
			t.Errorf("image2[%d].ID is empty; expected non-empty", i)
		}
		if img.CreatedAt == "" {
			t.Errorf("image2[%d].CreatedAt is empty; expected non-empty", i)
		}
		if img.OriginalImage != nil || img.ProcessedImage != nil {
			t.Errorf("image2[%d] binary fields should be nil when not selected", i)
		}
	}
}

func TestSQLite_GetImages_UnknownField(t *testing.T) {
	ds := newTestDB(t)
	_, err := ds.GetImages("nonexistent_field")
	if err == nil {
		t.Fatalf("expected error for unknown field, got nil")
	}
}

func TestSQLite_GetImages_AllFields(t *testing.T) {
	ds := newTestDB(t)

	id, err := ds.CreateImage([]byte("original"), []byte("processed"))
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}

	images, err := ds.GetImages()
	if err != nil {
		t.Fatalf("GetAllImages error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	img := images[0]
	if img.ID == "" {
		t.Errorf("ID is empty; expected non-empty")
	}
	if img.ID != id {
		t.Errorf("expected ID %q, got %q", id, img.ID)
	}
	if img.CreatedAt == "" {
		t.Errorf("CreatedAt is empty; expected non-empty")
	}
	if !bytes.Equal(img.OriginalImage, []byte("original")) {
		t.Errorf("OriginalImage mismatch: got %q", string(img.OriginalImage))
	}
	if !bytes.Equal(img.ProcessedImage, []byte("processed")) {
		t.Errorf("ProcessedImage mismatch: got %q", string(img.ProcessedImage))
	}
}

func TestSQLite_ProcessedLifecycle(t *testing.T) {
	ds := newTestDB(t)

	id, err := ds.CreateImage([]byte("orig"), nil) // processed NULL initially
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}

	// Initially processed image should not be available
	_, err = ds.GetProcessedImageByID(id)
	if err == nil {
		t.Fatalf("expected sql.ErrNoRows when processed_image is NULL, got nil")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	// Set processed image and verify retrieval
	processed := []byte("processed_data")
	if err := ds.SetProcessedImage(id, processed); err != nil {
		t.Fatalf("SetProcessedImage error: %v", err)
	}

	gotProcessed, err := ds.GetProcessedImageByID(id)
	if err != nil {
		t.Fatalf("GetProcessedImageByID error: %v", err)
	}
	if !bytes.Equal(gotProcessed, processed) {
		t.Fatalf("processed image mismatch: expected %q, got %q", string(processed), string(gotProcessed))
	}
}

func TestSQLite_GetOriginalImageByID(t *testing.T) {
	ds := newTestDB(t)

	id, err := ds.CreateImage([]byte("orig_data"), []byte("proc"))
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}

	orig, err := ds.GetOriginalImageByID(id)
	if err != nil {
		t.Fatalf("GetOriginalImageByID error: %v", err)
	}
	if !bytes.Equal(orig, []byte("orig_data")) {
		t.Fatalf("original image mismatch: expected %q, got %q", "orig_data", string(orig))
	}
}

func TestSQLite_DeleteImage(t *testing.T) {
	ds := newTestDB(t)

	id1, err := ds.CreateImage([]byte("a"), []byte("A"))
	if err != nil {
		t.Fatalf("CreateImage #1 error: %v", err)
	}
	id2, err := ds.CreateImage([]byte("b"), []byte("B"))
	if err != nil {
		t.Fatalf("CreateImage #2 error: %v", err)
	}

	if err := ds.DeleteImage(id1); err != nil {
		t.Fatalf("DeleteImage error: %v", err)
	}

	images, err := ds.GetImages("id")
	if err != nil {
		t.Fatalf("GetImages error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image after deletion, got %d", len(images))
	}
	if images[0].ID != id2 {
		t.Fatalf("expected remaining ID %q, got %q", id2, images[0].ID)
	}
}
