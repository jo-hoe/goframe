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
}

// NewFakeDatabase returns an empty FakeDatabase.
func NewFakeDatabase(imageBaseURL string) *FakeDatabase {
	if imageBaseURL == "" {
		imageBaseURL = "/images"
	}
	return &FakeDatabase{
		state:        rotationState{Images: make(map[string]imageMetadata)},
		imageBaseURL: imageBaseURL,
	}
}

func (f *FakeDatabase) Close() error { return nil }

func (f *FakeDatabase) CreateImage(_ context.Context, original, processed []byte, createdAt time.Time, source, afterID string) (string, error) {
	if original == nil {
		return "", fmt.Errorf("original image data cannot be nil")
	}
	if processed == nil {
		return "", fmt.Errorf("processed image data cannot be nil")
	}
	id, err := generateID()
	if err != nil {
		return "", err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state.Images == nil {
		f.state.Images = make(map[string]imageMetadata)
	}
	f.state.Images[id] = imageMetadata{CreatedAt: createdAt.UTC(), Source: source}
	f.state.OrderedIDs = insertIDAfter(f.state.OrderedIDs, id, afterID)
	return id, nil
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
