package database_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jo-hoe/goframe/internal/database"
)

// newTestRedisDB creates an in-process miniredis server and a RedisDatabase connected to it.
func newTestRedisDB(t *testing.T) database.DatabaseService {
	t.Helper()
	mr := miniredis.RunT(t)
	db, err := database.NewRedisDatabase(mr.Addr(), "", 0, "test-frame")
	if err != nil {
		t.Fatalf("NewRedisDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRedisDatabase_CreateAndGetImage(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	original := []byte("original-png-data")
	processed := []byte("processed-png-data")
	now := time.Now().UTC().Truncate(time.Second)

	id, err := db.CreateImage(ctx, original, processed, now, "xkcd", "")
	if err != nil {
		t.Fatalf("CreateImage: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	img, err := db.GetImageByID(ctx, id)
	if err != nil {
		t.Fatalf("GetImageByID: %v", err)
	}
	if img == nil {
		t.Fatal("expected image, got nil")
	}
	if img.ID != id {
		t.Errorf("ID: want %s, got %s", id, img.ID)
	}
	if !bytes.Equal(img.OriginalImage, original) {
		t.Errorf("OriginalImage mismatch")
	}
	if !bytes.Equal(img.ProcessedImage, processed) {
		t.Errorf("ProcessedImage mismatch")
	}
	if img.Source != "xkcd" {
		t.Errorf("Source: want xkcd, got %s", img.Source)
	}
	if !img.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: want %v, got %v", now, img.CreatedAt)
	}
}

func TestRedisDatabase_GetImages_OrderPreserved(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	id1, _ := db.CreateImage(ctx, []byte("o1"), []byte("p1"), now, "", "")
	id2, _ := db.CreateImage(ctx, []byte("o2"), []byte("p2"), now, "src", "")
	id3, _ := db.CreateImage(ctx, []byte("o3"), []byte("p3"), now, "", "")

	images, err := db.GetImages(ctx, "id")
	if err != nil {
		t.Fatalf("GetImages: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	if images[0].ID != id1 || images[1].ID != id2 || images[2].ID != id3 {
		t.Errorf("unexpected order: %v %v %v", images[0].ID, images[1].ID, images[2].ID)
	}
}

func TestRedisDatabase_DeleteImage(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	id, _ := db.CreateImage(ctx, []byte("o"), []byte("p"), now, "", "")

	if err := db.DeleteImage(ctx, id); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}

	img, err := db.GetImageByID(ctx, id)
	if err != nil {
		t.Fatalf("GetImageByID after delete: %v", err)
	}
	if img != nil {
		t.Errorf("expected nil after delete, got %+v", img)
	}

	images, _ := db.GetImages(ctx, "id")
	if len(images) != 0 {
		t.Errorf("expected 0 images after delete, got %d", len(images))
	}
}

func TestRedisDatabase_UpdateRanks(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	id1, _ := db.CreateImage(ctx, []byte("o1"), []byte("p1"), now, "", "")
	id2, _ := db.CreateImage(ctx, []byte("o2"), []byte("p2"), now, "", "")
	id3, _ := db.CreateImage(ctx, []byte("o3"), []byte("p3"), now, "", "")

	// Reverse the order
	if err := db.UpdateRanks(ctx, []string{id3, id2, id1}); err != nil {
		t.Fatalf("UpdateRanks: %v", err)
	}

	images, _ := db.GetImages(ctx, "id")
	if len(images) != 3 {
		t.Fatalf("expected 3 images after rerank, got %d", len(images))
	}
	if images[0].ID != id3 || images[1].ID != id2 || images[2].ID != id1 {
		t.Errorf("unexpected order after rerank: %v %v %v", images[0].ID, images[1].ID, images[2].ID)
	}
}

func TestRedisDatabase_GetImagesBySource(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	if _, err := db.CreateImage(ctx, []byte("o1"), []byte("p1"), now, "xkcd", ""); err != nil {
		t.Fatalf("CreateImage: %v", err)
	}
	if _, err := db.CreateImage(ctx, []byte("o2"), []byte("p2"), now, "", ""); err != nil {
		t.Fatalf("CreateImage: %v", err)
	}
	if _, err := db.CreateImage(ctx, []byte("o3"), []byte("p3"), now, "xkcd", ""); err != nil {
		t.Fatalf("CreateImage: %v", err)
	}

	images, err := db.GetImagesBySource(ctx, "xkcd")
	if err != nil {
		t.Fatalf("GetImagesBySource: %v", err)
	}
	if len(images) != 2 {
		t.Errorf("expected 2 xkcd images, got %d", len(images))
	}
	for _, img := range images {
		if img.Source != "xkcd" {
			t.Errorf("expected source xkcd, got %s", img.Source)
		}
	}
}

func TestRedisDatabase_GetCurrentImageID_FallbackToFirst(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	id1, _ := db.CreateImage(ctx, []byte("o1"), []byte("p1"), now, "", "")
	if _, err := db.CreateImage(ctx, []byte("o2"), []byte("p2"), now, "", ""); err != nil {
		t.Fatalf("CreateImage: %v", err)
	}

	// No operator has set the rotation key, so it falls back to the first image.
	currentID, err := db.GetCurrentImageID(ctx)
	if err != nil {
		t.Fatalf("GetCurrentImageID: %v", err)
	}
	if currentID != id1 {
		t.Errorf("expected first image %s, got %s", id1, currentID)
	}
}

func TestRedisDatabase_CreateImage_NilInputs(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)
	now := time.Now().UTC()

	_, err := db.CreateImage(ctx, nil, []byte("p"), now, "", "")
	if err == nil {
		t.Error("expected error for nil original")
	}

	_, err = db.CreateImage(ctx, []byte("o"), nil, now, "", "")
	if err == nil {
		t.Error("expected error for nil processed")
	}
}

func TestRedisDatabase_CreateImage_AfterID(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	id1, _ := db.CreateImage(ctx, []byte("o1"), []byte("p1"), now, "", "")
	id2, _ := db.CreateImage(ctx, []byte("o2"), []byte("p2"), now, "", "")
	id3, _ := db.CreateImage(ctx, []byte("o3"), []byte("p3"), now, "", "")

	// Insert id4 after id1 → expected order: [id1, id4, id2, id3]
	id4, err := db.CreateImage(ctx, []byte("o4"), []byte("p4"), now, "", id1)
	if err != nil {
		t.Fatalf("CreateImage afterID: %v", err)
	}

	images, err := db.GetImages(ctx, "id")
	if err != nil {
		t.Fatalf("GetImages: %v", err)
	}
	if len(images) != 4 {
		t.Fatalf("expected 4 images, got %d", len(images))
	}
	if images[0].ID != id1 || images[1].ID != id4 || images[2].ID != id2 || images[3].ID != id3 {
		t.Errorf("unexpected order: got [%s %s %s %s], want [%s %s %s %s]",
			images[0].ID, images[1].ID, images[2].ID, images[3].ID,
			id1, id4, id2, id3)
	}
}

func TestRedisDatabase_CreateImage_AfterID_AtEnd(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	now := time.Now().UTC()
	id1, _ := db.CreateImage(ctx, []byte("o1"), []byte("p1"), now, "", "")
	id2, _ := db.CreateImage(ctx, []byte("o2"), []byte("p2"), now, "", "")

	// Insert id3 after id2 (the last) → expected order: [id1, id2, id3]
	id3, err := db.CreateImage(ctx, []byte("o3"), []byte("p3"), now, "", id2)
	if err != nil {
		t.Fatalf("CreateImage afterID at end: %v", err)
	}

	images, err := db.GetImages(ctx, "id")
	if err != nil {
		t.Fatalf("GetImages: %v", err)
	}
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	if images[0].ID != id1 || images[1].ID != id2 || images[2].ID != id3 {
		t.Errorf("unexpected order: got [%s %s %s], want [%s %s %s]",
			images[0].ID, images[1].ID, images[2].ID, id1, id2, id3)
	}
}

func TestRedisDatabase_CreateImage_ConcurrentUploads(t *testing.T) {
	ctx := context.Background()
	db := newTestRedisDB(t)

	const n = 10
	now := time.Now().UTC()

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := db.CreateImage(ctx, []byte("orig"), []byte("proc"), now, "", "")
			if err != nil {
				t.Errorf("concurrent CreateImage: %v", err)
			}
		}()
	}
	wg.Wait()

	images, err := db.GetImages(ctx, "id")
	if err != nil {
		t.Fatalf("GetImages: %v", err)
	}
	if len(images) != n {
		t.Fatalf("expected %d images, got %d", n, len(images))
	}

	// Verify all IDs are unique (no duplicate sorted-set members).
	seen := make(map[string]bool, n)
	for _, img := range images {
		if seen[img.ID] {
			t.Errorf("duplicate image ID in results: %s", img.ID)
		}
		seen[img.ID] = true
	}
	if len(seen) != n {
		t.Errorf("expected %d unique IDs, got %d", n, len(seen))
	}
}
