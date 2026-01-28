package database

import (
	"bytes"
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
		if img.Rank != "" {
			t.Errorf("image[%d].Rank is not empty; expected empty when not selected", i)
		}
		seen[img.ID] = true
	}
	if !seen[id1] || !seen[id2] {
		t.Fatalf("expected IDs %q and %q to be present in results, got %v", id1, id2, seen)
	}

	// Request ID and rank
	images2, err := ds.GetImages("id", "rank")
	if err != nil {
		t.Fatalf("GetImages(id, rank) error: %v", err)
	}
	if len(images2) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images2))
	}
	for i, img := range images2 {
		if img.ID == "" {
			t.Errorf("image2[%d].ID is empty; expected non-empty", i)
		}
		if img.Rank == "" {
			t.Errorf("image2[%d].Rank is empty; expected non-empty", i)
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
	if img.Rank == "" {
		t.Errorf("Rank is empty; expected non-empty")
	}
	if !bytes.Equal(img.OriginalImage, []byte("original")) {
		t.Errorf("OriginalImage mismatch: got %q", string(img.OriginalImage))
	}
	if !bytes.Equal(img.ProcessedImage, []byte("processed")) {
		t.Errorf("ProcessedImage mismatch: got %q", string(img.ProcessedImage))
	}
}

func TestSQLite_GetImageByID(t *testing.T) {
	ds := newTestDB(t)

	id, err := ds.CreateImage([]byte("orig"), []byte("proc"))
	if err != nil {
		t.Fatalf("CreateImage error: %v", err)
	}

	img, err := ds.GetImageByID(id)
	if err != nil {
		t.Fatalf("GetImageByID error: %v", err)
	}
	if img == nil {
		t.Fatalf("GetImageByID returned nil; expected image")
	}
	if img.ID != id {
		t.Errorf("expected ID %q, got %q", id, img.ID)
	}
	if !bytes.Equal(img.OriginalImage, []byte("orig")) {
		t.Errorf("OriginalImage mismatch: got %q", string(img.OriginalImage))
	}
	if !bytes.Equal(img.ProcessedImage, []byte("proc")) {
		t.Errorf("ProcessedImage mismatch: got %q", string(img.ProcessedImage))
	}

	// Test non-existent ID
	nonExistentID := "non-existent-id"
	img2, err := ds.GetImageByID(nonExistentID)
	if err != nil {
		t.Fatalf("GetImageByID(non-existent) error: %v", err)
	}
	if img2 != nil {
		t.Fatalf("GetImageByID(non-existent) returned non-nil; expected nil")
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
