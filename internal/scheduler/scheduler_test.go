package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// minimalPNG returns a 1x1 white PNG as bytes for use in tests.
func minimalPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic("minimalPNG: " + err.Error())
	}
	return buf.Bytes()
}

// staticSource is a test ImageSource that returns fixed bytes.
type staticSource struct {
	name string
	data []byte
	err  error
}

func (s *staticSource) Name() string                             { return s.name }
func (s *staticSource) Fetch(_ context.Context) ([]byte, error) { return s.data, s.err }

// goframeTestServer simulates the goframe REST API for image scheduler integration tests.
type goframeTestServer struct {
	images []apiImageItem
	// uploadedSource records the source form field value from the last upload.
	uploadedSource string
	// deletedIDs records all deleted image IDs in order.
	deletedIDs []string
}

func (g *goframeTestServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/images", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(g.images)
	})
	mux.HandleFunc("/api/image", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, "bad multipart form", http.StatusBadRequest)
			return
		}
		g.uploadedSource = r.FormValue("source")
		newID := "new-id-" + g.uploadedSource
		g.images = append(g.images, apiImageItem{
			ID:        newID,
			CreatedAt: time.Now(),
			Source:    g.uploadedSource,
		})
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/images/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/images/")
		g.deletedIDs = append(g.deletedIDs, id)
		updated := g.images[:0]
		for _, img := range g.images {
			if img.ID != id {
				updated = append(updated, img)
			}
		}
		g.images = updated
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func newGoframeTestServer(images []apiImageItem) (*httptest.Server, *goframeTestServer) {
	state := &goframeTestServer{images: images}
	srv := httptest.NewServer(state.handler())
	return srv, state
}

// --- RunOnce tests ---

func TestRunOnce_UploadsWhenNoUnmanagedImages(t *testing.T) {
	srv, state := newGoframeTestServer(nil)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:              srv.URL,
		SourceName:                  "test-source",
		KeepCount:                   1,
		DrainIfUnmanagedImagesExceed: 0,
		Source:                      &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "test-source" {
		t.Errorf("expected source=test-source, got %q", state.uploadedSource)
	}
	if len(state.images) != 1 {
		t.Errorf("expected 1 image after upload, got %d", len(state.images))
	}
}

func TestRunOnce_DrainsOwnImagesWhenUnmanagedCountExceedsThreshold(t *testing.T) {
	// Two unmanaged images exceed threshold=1; one own image should be drained.
	initialImages := []apiImageItem{
		{ID: "other-1", Source: ""},
		{ID: "other-2", Source: "other-source"},
		{ID: "own-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:               srv.URL,
		SourceName:                   "test-source",
		KeepCount:                    1,
		DrainIfUnmanagedImagesExceed: 1, // threshold=1, unmanaged count=2 → drain
		Source:                       &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "" {
		t.Errorf("expected no upload when threshold exceeded, but got upload with source %q", state.uploadedSource)
	}
	if len(state.deletedIDs) != 1 || state.deletedIDs[0] != "own-1" {
		t.Errorf("expected own-1 to be drained, got deletedIDs=%v", state.deletedIDs)
	}
}

func TestRunOnce_ActsWhenUnmanagedCountAtThreshold(t *testing.T) {
	initialImages := []apiImageItem{
		{ID: "other-1", Source: ""},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:              srv.URL,
		SourceName:                  "test-source",
		KeepCount:                   1,
		DrainIfUnmanagedImagesExceed: 1, // threshold=1, unmanaged count=1 → act
		Source:                      &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "test-source" {
		t.Errorf("expected upload when unmanaged count equals threshold, got %q", state.uploadedSource)
	}
}

func TestRunOnce_PrunesExcessOwnImages(t *testing.T) {
	// Two existing image scheduler images; keepCount=1 → oldest should be pruned after upload.
	initialImages := []apiImageItem{
		{ID: "sched-old-1", Source: "test-source"},
		{ID: "sched-old-2", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:              srv.URL,
		SourceName:                  "test-source",
		KeepCount:                   1,
		DrainIfUnmanagedImagesExceed: 0,
		Source:                      &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	// After upload there are 3 image scheduler images; keepCount=1 means 2 should be deleted.
	if len(state.deletedIDs) != 2 {
		t.Errorf("expected 2 deletions, got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	if len(state.images) != 1 {
		t.Errorf("expected 1 remaining image, got %d", len(state.images))
	}
}

func TestRunOnce_OnlyPrunesOwnImages(t *testing.T) {
	// Mix of unmanaged and other-source images; image scheduler must not touch them.
	initialImages := []apiImageItem{
		{ID: "unmanaged-1", Source: ""},
		{ID: "other-source-1", Source: "other"},
		{ID: "sched-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:              srv.URL,
		SourceName:                  "test-source",
		KeepCount:                   1,
		DrainIfUnmanagedImagesExceed: 5, // high threshold so image scheduler always acts
		Source:                      &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	// After upload: 2 test-source images, keepCount=1 → 1 pruned
	if len(state.deletedIDs) != 1 {
		t.Errorf("expected 1 deletion (own image only), got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	// The deleted ID must be the original image scheduler image, not unmanaged or other-source.
	if state.deletedIDs[0] != "sched-1" {
		t.Errorf("expected sched-1 to be deleted, got %q", state.deletedIDs[0])
	}
}

func TestRunOnce_SourceFetchError(t *testing.T) {
	// Source returns an error — RunOnce should propagate it without uploading or deleting.
	srv, state := newGoframeTestServer(nil)
	defer srv.Close()

	fetchErr := io.EOF // arbitrary sentinel error
	cfg := Config{
		GoframeBaseURL:              srv.URL,
		SourceName:                  "test-source",
		KeepCount:                   1,
		DrainIfUnmanagedImagesExceed: 0,
		Source:                      &staticSource{name: "test-source", err: fetchErr},
	}

	err := RunOnce(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when source fetch fails, got nil")
	}
	if state.uploadedSource != "" {
		t.Error("expected no upload when source fetch fails")
	}
}

// --- Helper function tests ---

func TestCountUnmanagedImages(t *testing.T) {
	images := []apiImageItem{
		{ID: "1", Source: ""},
		{ID: "2", Source: "xkcd"},
		{ID: "3", Source: ""},
		{ID: "4", Source: "other"},
	}
	// sourceName="xkcd": images 1, 3, 4 are unmanaged → 3
	if got := countUnmanagedImages(images, "xkcd"); got != 3 {
		t.Errorf("expected 3 unmanaged images, got %d", got)
	}
}

func TestCountUnmanagedImages_Empty(t *testing.T) {
	if got := countUnmanagedImages(nil, "xkcd"); got != 0 {
		t.Errorf("expected 0 for nil slice, got %d", got)
	}
}

func TestFilterBySource(t *testing.T) {
	images := []apiImageItem{
		{ID: "1", Source: "xkcd"},
		{ID: "2", Source: ""},
		{ID: "3", Source: "xkcd"},
		{ID: "4", Source: "other"},
	}
	got := filterBySource(images, "xkcd")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	for _, img := range got {
		if img.Source != "xkcd" {
			t.Errorf("unexpected source %q in filtered result", img.Source)
		}
	}
}
