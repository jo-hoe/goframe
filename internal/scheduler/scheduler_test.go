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

func TestRunOnce_Uploads(t *testing.T) {
	srv, state := newGoframeTestServer(nil)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		KeepCount:      1,
		Source:         &staticSource{name: "test-source", data: minimalPNG()},
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

func TestRunOnce_PrunesExcessOwnImages(t *testing.T) {
	// Two existing own images; keepCount=1 → oldest should be pruned after upload.
	initialImages := []apiImageItem{
		{ID: "sched-old-1", Source: "test-source"},
		{ID: "sched-old-2", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		KeepCount:      1,
		Source:         &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	// After upload there are 3 own images; keepCount=1 means 2 should be deleted.
	if len(state.deletedIDs) != 2 {
		t.Errorf("expected 2 deletions, got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	if len(state.images) != 1 {
		t.Errorf("expected 1 remaining image, got %d", len(state.images))
	}
}

func TestRunOnce_OnlyPrunesOwnImages(t *testing.T) {
	// Images from other sources must not be touched during own-image pruning.
	initialImages := []apiImageItem{
		{ID: "unmanaged-1", Source: ""},
		{ID: "other-source-1", Source: "other"},
		{ID: "sched-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		KeepCount:      1,
		Source:         &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	// After upload: 2 test-source images, keepCount=1 → 1 pruned, and it must be the original own image.
	if len(state.deletedIDs) != 1 {
		t.Errorf("expected 1 deletion (own image only), got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	if state.deletedIDs[0] != "sched-1" {
		t.Errorf("expected sched-1 to be deleted, got %q", state.deletedIDs[0])
	}
}

func TestRunOnce_SourceFetchError(t *testing.T) {
	// Source returns an error — RunOnce should propagate it without uploading or deleting.
	srv, state := newGoframeTestServer(nil)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		KeepCount:      1,
		Source:         &staticSource{name: "test-source", err: io.EOF},
	}

	err := RunOnce(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when source fetch fails, got nil")
	}
	if state.uploadedSource != "" {
		t.Error("expected no upload when source fetch fails")
	}
}

func TestRunOnce_ExclusionGroup_EvictsPeers(t *testing.T) {
	// On successful upload, all images from other group members should be deleted.
	initialImages := []apiImageItem{
		{ID: "peer-img-1", Source: "pusheen"},
		{ID: "peer-img-2", Source: "oatmeal"},
		{ID: "own-img-1", Source: "xkcd"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "xkcd",
		KeepCount:      1,
		ExclusionGroup: "daily-wallpaper",
		GroupMembers:   []string{"xkcd", "pusheen", "oatmeal"},
		Source:         &staticSource{name: "xkcd", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "xkcd" {
		t.Errorf("expected upload from xkcd, got %q", state.uploadedSource)
	}
	// peer-img-1 (pusheen) and peer-img-2 (oatmeal) must be evicted; own-img-1 pruned to keepCount=1.
	if len(state.deletedIDs) != 3 {
		t.Errorf("expected 3 deletions (2 peers + 1 own excess), got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	deletedSet := make(map[string]bool, len(state.deletedIDs))
	for _, id := range state.deletedIDs {
		deletedSet[id] = true
	}
	if !deletedSet["peer-img-1"] {
		t.Error("expected peer-img-1 (pusheen) to be evicted")
	}
	if !deletedSet["peer-img-2"] {
		t.Error("expected peer-img-2 (oatmeal) to be evicted")
	}
	if !deletedSet["own-img-1"] {
		t.Error("expected own-img-1 to be pruned (exceeds keepCount=1)")
	}
}

func TestRunOnce_ExclusionGroup_DoesNotEvictNonMembers(t *testing.T) {
	// Images from sources outside the group must not be touched.
	initialImages := []apiImageItem{
		{ID: "peer-img-1", Source: "pusheen"},
		{ID: "external-img", Source: "some-other-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "xkcd",
		KeepCount:      1,
		ExclusionGroup: "daily-wallpaper",
		GroupMembers:   []string{"xkcd", "pusheen"},
		Source:         &staticSource{name: "xkcd", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	deletedSet := make(map[string]bool, len(state.deletedIDs))
	for _, id := range state.deletedIDs {
		deletedSet[id] = true
	}
	if deletedSet["external-img"] {
		t.Error("external-img (not a group member) must not be deleted")
	}
	if !deletedSet["peer-img-1"] {
		t.Error("expected peer-img-1 (pusheen group member) to be evicted")
	}
}

func TestRunOnce_NoExclusionGroup_DoesNotEvictPeers(t *testing.T) {
	// Without an exclusion group, images from other sources are left alone.
	initialImages := []apiImageItem{
		{ID: "other-img", Source: "pusheen"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "xkcd",
		KeepCount:      1,
		Source:         &staticSource{name: "xkcd", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	for _, id := range state.deletedIDs {
		if id == "other-img" {
			t.Error("other-img must not be deleted when no exclusion group is set")
		}
	}
}

// --- Helper function tests ---

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
