package database

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FakeDatabase is an in-memory DatabaseService for use in tests.
// It is safe for concurrent use.
type FakeDatabase struct {
	mu           sync.Mutex
	state        rotationState
	imageBaseURL string
	// objects is an in-memory stand-in for the S3 object store, keyed by object
	// key. It holds the processed/original blobs and the status marker objects,
	// so the derived-status logic can be exercised exactly as in production.
	objects map[string][]byte
	// storeUploadErr, when non-nil, is returned by StoreUpload to exercise the
	// upload-persist failure path in tests.
	storeUploadErr error
}

// NewFakeDatabase returns an empty FakeDatabase.
func NewFakeDatabase(imageBaseURL string) *FakeDatabase {
	if imageBaseURL == "" {
		imageBaseURL = "/images"
	}
	return &FakeDatabase{
		state:        rotationState{Images: make(map[string]imageMetadata)},
		imageBaseURL: imageBaseURL,
		objects:      make(map[string][]byte),
	}
}

func (f *FakeDatabase) Close() error { return nil }

func (f *FakeDatabase) CreateImage(_ context.Context, id string, original, processed []byte, createdAt time.Time, source, afterID string) error {
	if err := ValidateContentID(id); err != nil {
		return err
	}
	if original == nil {
		return fmt.Errorf("original image data cannot be nil")
	}
	if processed == nil {
		return fmt.Errorf("processed image data cannot be nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state.Images == nil {
		f.state.Images = make(map[string]imageMetadata)
	}
	f.state.Images[id] = imageMetadata{CreatedAt: createdAt.UTC(), Source: source}
	if !containsID(f.state.OrderedIDs, id) {
		f.state.OrderedIDs = insertIDAfter(f.state.OrderedIDs, id, afterID)
	}
	f.objects[imageOriginalKey(id)] = original
	f.objects[imageProcessedKey(id)] = processed
	return nil
}

// StoreUpload persists the raw uploaded bytes under the upload key.
func (f *FakeDatabase) StoreUpload(_ context.Context, id string, raw []byte) error {
	if err := ValidateContentID(id); err != nil {
		return err
	}
	if raw == nil {
		return fmt.Errorf("upload data cannot be nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.storeUploadErr != nil {
		return f.storeUploadErr
	}
	f.objects[imageUploadKey(id)] = raw
	return nil
}

// SetStoreUploadErr configures StoreUpload to fail with the given error. It is a
// test hook for exercising the upload-persist failure path.
func (f *FakeDatabase) SetStoreUploadErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeUploadErr = err
}

// HasUpload reports whether the raw upload blob exists for the given ID. It is a
// test hook so callers outside this package can assert early durability without
// touching the internal object store.
func (f *FakeDatabase) HasUpload(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[imageUploadKey(id)]
	return ok
}

// HasProcessed reports whether the processed blob exists for the given ID.
func (f *FakeDatabase) HasProcessed(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[imageProcessedKey(id)]
	return ok
}

// HasOriginal reports whether the normalized original blob exists for the given ID.
func (f *FakeDatabase) HasOriginal(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[imageOriginalKey(id)]
	return ok
}

// objectExists reports whether an object key is present in the fake store.
func (f *FakeDatabase) objectExists(_ context.Context, key string) (bool, error) {
	_, ok := f.objects[key]
	return ok, nil
}

// getObject returns the object bytes, or (nil, nil) when absent, mirroring the
// s3Client.GetObject contract.
func (f *FakeDatabase) getObject(_ context.Context, key string) ([]byte, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, nil
	}
	return data, nil
}

// ImageExists reports whether a processed blob exists for the given ID.
func (f *FakeDatabase) ImageExists(ctx context.Context, id string) (bool, error) {
	if err := ValidateContentID(id); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objectExists(ctx, imageProcessedKey(id))
}

// GetUploadState returns the derived processing state for a content-addressed ID.
func (f *FakeDatabase) GetUploadState(ctx context.Context, id string) (*UploadState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return deriveUploadState(ctx, id, DefaultProcessingMarkerTTL, f.objectExists, f.getObject)
}

// MarkProcessing writes the "processing" status marker for the given ID. The
// marker carries a timestamp so a stale marker (left by a crashed worker) can
// be detected by deriveUploadState.
func (f *FakeDatabase) MarkProcessing(_ context.Context, id string) error {
	if err := ValidateContentID(id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[imageProcessingMarkerKey(id)] = newProcessingMarker()
	return nil
}

// MarkFailed writes the "failed" status marker (with errMsg) for the given ID.
func (f *FakeDatabase) MarkFailed(_ context.Context, id, errMsg string) error {
	if err := ValidateContentID(id); err != nil {
		return err
	}
	data, err := newFailureMarker(errMsg)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, imageProcessingMarkerKey(id))
	f.objects[imageFailedMarkerKey(id)] = data
	return nil
}

// ClearStatusMarkers removes the processing and failed markers for the given ID.
func (f *FakeDatabase) ClearStatusMarkers(_ context.Context, id string) error {
	if err := ValidateContentID(id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, imageProcessingMarkerKey(id))
	delete(f.objects, imageFailedMarkerKey(id))
	return nil
}

func (f *FakeDatabase) GetImageMetadata(_ context.Context) ([]*Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	images := make([]*Image, 0, len(f.state.OrderedIDs))
	for _, id := range f.state.OrderedIDs {
		meta := f.state.Images[id]
		images = append(images, &Image{ID: id, CreatedAt: meta.CreatedAt, Source: meta.Source})
	}
	return images, nil
}

func (f *FakeDatabase) GetImageByID(_ context.Context, id string) (*Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	meta, ok := f.state.Images[id]
	if !ok {
		return nil, fmt.Errorf("image not found: %s", id)
	}
	return &Image{ID: id, CreatedAt: meta.CreatedAt, Source: meta.Source}, nil
}

func (f *FakeDatabase) DeleteImage(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.state.Images[id]; !ok {
		return fmt.Errorf("image not found: %s", id)
	}
	delete(f.state.Images, id)
	f.state.OrderedIDs = removeID(f.state.OrderedIDs, id)
	delete(f.objects, imageOriginalKey(id))
	delete(f.objects, imageProcessedKey(id))
	delete(f.objects, imageUploadKey(id))
	delete(f.objects, imageProcessingMarkerKey(id))
	delete(f.objects, imageFailedMarkerKey(id))
	return nil
}

func (f *FakeDatabase) UpdateOrder(_ context.Context, order []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.state.OrderedIDs = order
	return nil
}

func (f *FakeDatabase) GetRotationOrderedIDs(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ids := make([]string, len(f.state.OrderedIDs))
	copy(ids, f.state.OrderedIDs)
	return ids, nil
}

func (f *FakeDatabase) GetCurrentImageID(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.state.OrderedIDs) == 0 {
		return "", fmt.Errorf("no images")
	}
	return f.state.OrderedIDs[0], nil
}

func (f *FakeDatabase) GetCurrentImageURL(_ context.Context, id, variant string) (string, error) {
	switch variant {
	case "processed":
		return f.imageBaseURL + "/" + id + "/processed.png", nil
	default:
		return f.imageBaseURL + "/" + id + "/original.png", nil
	}
}

func (f *FakeDatabase) GetLastRotatedTime(_ context.Context) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state.LastRotated.IsZero() {
		return time.Time{}, fmt.Errorf("last-rotated key not set")
	}
	return f.state.LastRotated, nil
}
