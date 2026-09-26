package core

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/jo-hoe/goframe/internal/config"
	"github.com/jo-hoe/goframe/internal/database"
	"github.com/jo-hoe/goframe/internal/imagevalidation"
)

// newTestCoreService builds a CoreService backed by an in-memory fake database
// and no processing commands (so applyPipeline only PNG-converts).
func newTestCoreService(t *testing.T) (*CoreService, *database.FakeDatabase) {
	t.Helper()
	fake := database.NewFakeDatabase("/images")
	return &CoreService{
		config:          &config.ServiceConfig{MaxUploadBytes: config.DefaultMaxUploadBytes, MaxConcurrentProcessing: config.DefaultMaxConcurrentProcessing},
		databaseService: fake,
		tzLoc:           time.UTC,
		sem:             make(chan struct{}, config.DefaultMaxConcurrentProcessing),
	}, fake
}

// tinyPNG returns the bytes of a minimal valid PNG image.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestSubmitImage_ProcessesAndSucceeds(t *testing.T) {
	svc, _ := newTestCoreService(t)
	ctx := context.Background()
	data := tinyPNG(t)

	state, err := svc.SubmitImage(ctx, data, "")
	if err != nil {
		t.Fatalf("SubmitImage: %v", err)
	}
	if state.Status != database.StatusProcessing {
		t.Fatalf("initial status = %q, want processing", state.Status)
	}
	if state.ID != database.ContentID(data) {
		t.Fatalf("id = %q, want content hash", state.ID)
	}

	svc.WaitForProcessing()

	final, err := svc.GetUploadState(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetUploadState: %v", err)
	}
	if final.Status != database.StatusSucceeded {
		t.Fatalf("final status = %q, want succeeded", final.Status)
	}
}

func TestSubmitImage_IdempotentRepeat(t *testing.T) {
	svc, fake := newTestCoreService(t)
	ctx := context.Background()
	data := tinyPNG(t)

	first, err := svc.SubmitImage(ctx, data, "")
	if err != nil {
		t.Fatalf("first SubmitImage: %v", err)
	}
	svc.WaitForProcessing()

	// Re-submit identical bytes: must resolve to the same ID and start no new work.
	second, err := svc.SubmitImage(ctx, data, "")
	if err != nil {
		t.Fatalf("second SubmitImage: %v", err)
	}
	svc.WaitForProcessing()

	if first.ID != second.ID {
		t.Fatalf("ids differ: %q != %q", first.ID, second.ID)
	}
	if second.Status != database.StatusSucceeded {
		t.Fatalf("repeat status = %q, want succeeded", second.Status)
	}

	ids, err := fake.GetRotationOrderedIDs(ctx)
	if err != nil {
		t.Fatalf("GetRotationOrderedIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("rotation has %d images, want 1 (no duplicate)", len(ids))
	}
}

func TestSubmitImage_RejectsInvalid(t *testing.T) {
	svc, _ := newTestCoreService(t)
	ctx := context.Background()

	if _, err := svc.SubmitImage(ctx, nil, ""); err == nil {
		t.Fatal("expected error for empty upload")
	}

	_, err := svc.SubmitImage(ctx, []byte("not an image"), "")
	if err == nil {
		t.Fatal("expected error for non-image bytes")
	}
	if _, ok := imagevalidation.AsValidationError(err); !ok {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	svc.WaitForProcessing()
}

// TestSubmitImage_StoresRawUploadBeforeReturn asserts the raw upload blob is
// durable the instant SubmitImage returns (stored synchronously before 202),
// while normalization/processing outputs are produced only in the background.
func TestSubmitImage_StoresRawUploadBeforeReturn(t *testing.T) {
	svc, fake := newTestCoreService(t)
	ctx := context.Background()
	data := tinyPNG(t)

	state, err := svc.SubmitImage(ctx, data, "")
	if err != nil {
		t.Fatalf("SubmitImage: %v", err)
	}

	// The raw upload is persisted synchronously before the (would-be) 202.
	if !fake.HasUpload(state.ID) {
		t.Fatal("raw upload blob missing immediately after SubmitImage returned")
	}

	svc.WaitForProcessing()

	// After background processing: normalized + processed blobs and one rotation entry.
	if !fake.HasOriginal(state.ID) {
		t.Fatal("original.png missing after processing")
	}
	if !fake.HasProcessed(state.ID) {
		t.Fatal("processed.png missing after processing")
	}
	final, err := svc.GetUploadState(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetUploadState: %v", err)
	}
	if final.Status != database.StatusSucceeded {
		t.Fatalf("final status = %q, want succeeded", final.Status)
	}
	ids, err := fake.GetRotationOrderedIDs(ctx)
	if err != nil {
		t.Fatalf("GetRotationOrderedIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("rotation has %d images, want 1", len(ids))
	}
}

// TestSubmitImage_UploadPersistFails asserts that when persisting the raw upload
// fails, SubmitImage returns the error, clears the processing marker, stores no
// blobs, and spawns no background worker.
func TestSubmitImage_UploadPersistFails(t *testing.T) {
	svc, fake := newTestCoreService(t)
	ctx := context.Background()
	data := tinyPNG(t)

	fake.SetStoreUploadErr(errors.New("boom"))

	if _, err := svc.SubmitImage(ctx, data, ""); err == nil {
		t.Fatal("expected SubmitImage to fail when StoreUpload fails")
	}

	id := database.ContentID(data)

	// No upload blob was stored, and no background worker should be running.
	svc.WaitForProcessing()
	if fake.HasUpload(id) {
		t.Fatal("upload blob present despite StoreUpload failure")
	}
	if fake.HasProcessed(id) {
		t.Fatal("processed.png present despite StoreUpload failure")
	}

	// The processing marker was cleared, so status derives back to unknown.
	state, err := svc.GetUploadState(ctx, id)
	if err != nil {
		t.Fatalf("GetUploadState: %v", err)
	}
	if state.Status != database.StatusUnknown {
		t.Fatalf("status = %q, want unknown (marker cleared)", state.Status)
	}
}
