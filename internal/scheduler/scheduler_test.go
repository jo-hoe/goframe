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
//
// It mirrors the real async upload contract: POST /api/image returns 202 with a
// JSON body and does NOT immediately register the image in the list. The image
// only appears once its status flips to "succeeded", which happens after
// processingPolls status polls — modelling background processing latency. This
// is what exercises waitForTerminal and guards against the eviction/prune race.
type goframeTestServer struct {
	images []apiImageItem
	// uploadedSource records the source form field value from the last upload.
	uploadedSource string
	// deletedIDs records all deleted image IDs in order.
	deletedIDs []string
	// processingPolls is the number of status polls that return "processing"
	// before the upload flips to "succeeded" (and the image is registered).
	processingPolls int
	// failUpload, when true, makes the uploaded image flip to "failed" instead
	// of "succeeded" after processingPolls polls (and never registers it).
	failUpload bool

	// pendingID is the ID of the upload currently being processed.
	pendingID string
	// pendingSource is the source of the upload currently being processed.
	pendingSource string
	// pollCount counts status polls for the pending upload.
	pollCount int
}

func (g *goframeTestServer) uploadResponseBody() uploadResponse {
	return uploadResponse{
		ID:        g.pendingID,
		Status:    string(statusProcessing),
		StatusURL: "/api/images/" + g.pendingID + "/status",
	}
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
		if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // test server with MaxBytesReader
			http.Error(w, "bad multipart form", http.StatusBadRequest)
			return
		}
		g.uploadedSource = r.FormValue("source")
		g.pendingSource = g.uploadedSource
		g.pendingID = "new-id-" + g.uploadedSource
		g.pollCount = 0

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(g.uploadResponseBody())
	})
	mux.HandleFunc("/api/images/", g.handleImageSubpath)
	return mux
}

// handleImageSubpath routes GET /api/images/{id}/status and DELETE /api/images/{id}.
func (g *goframeTestServer) handleImageSubpath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/images/")
	if id, ok := strings.CutSuffix(rest, "/status"); ok {
		g.handleStatus(w, r, id)
		return
	}
	g.handleDelete(w, r, rest)
}

func (g *goframeTestServer) handleStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := statusProcessing
	if id == g.pendingID {
		g.pollCount++
		if g.pollCount > g.processingPolls {
			if g.failUpload {
				status = statusFailed
			} else {
				status = statusSucceeded
				g.registerPending()
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(uploadStateResponse{ID: id, Status: string(status)})
}

// registerPending appends the pending upload to the image list, mimicking the
// server's CreateImage at the end of background processing. It is idempotent.
func (g *goframeTestServer) registerPending() {
	if g.pendingID == "" {
		return
	}
	for _, img := range g.images {
		if img.ID == g.pendingID {
			return
		}
	}
	g.images = append(g.images, apiImageItem{
		ID:        g.pendingID,
		CreatedAt: time.Now(),
		Source:    g.pendingSource,
	})
}

func (g *goframeTestServer) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	g.deletedIDs = append(g.deletedIDs, id)
	updated := g.images[:0]
	for _, img := range g.images {
		if img.ID != id {
			updated = append(updated, img)
		}
	}
	g.images = updated
	w.WriteHeader(http.StatusNoContent)
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
	// Two existing own images; always keeps 1 → oldest should be pruned after upload.
	initialImages := []apiImageItem{
		{ID: "sched-old-1", Source: "test-source"},
		{ID: "sched-old-2", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		Source:         &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	// After upload there are 3 own images; keeps 1 means 2 should be deleted.
	if len(state.deletedIDs) != 2 {
		t.Errorf("expected 2 deletions, got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	if len(state.images) != 1 {
		t.Errorf("expected 1 remaining image, got %d", len(state.images))
	}
}

func TestRunOnce_OnlyPrunesOwnImages(t *testing.T) {
	// External images must not be touched during own-image pruning (onExternalImages=ignore).
	initialImages := []apiImageItem{
		{ID: "external-1", Source: ""},
		{ID: "other-source-1", Source: "other"},
		{ID: "sched-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:   srv.URL,
		SourceName:       "test-source",
		OnExternalImages: OnExternalImagesIgnore,
		Source:           &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	// After upload: 2 test-source images, keeps 1 → 1 pruned, and it must be the original own image.
	if len(state.deletedIDs) != 1 {
		t.Errorf("expected 1 deletion (own image only), got %d: %v", len(state.deletedIDs), state.deletedIDs)
	}
	if state.deletedIDs[0] != "sched-1" {
		t.Errorf("expected sched-1 to be deleted, got %q", state.deletedIDs[0])
	}
}

func TestRunOnce_SourceFetchError(t *testing.T) {
	srv, state := newGoframeTestServer(nil)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
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

func TestRunOnce_WaitsForAsyncUploadBeforePruning(t *testing.T) {
	// Regression for the async-upload eviction race: POST /api/image returns 202
	// and the new image is registered in the list only after several status
	// polls (background processing). RunOnce must wait for "succeeded" before it
	// lists + prunes; otherwise it would see only the pre-existing own image,
	// prune nothing, and end with two own images.
	initialImages := []apiImageItem{
		{ID: "sched-old-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	state.processingPolls = 2 // image appears only on the 3rd status poll
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		Source:         &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	own := filterBySource(state.images, "test-source")
	if len(own) != 1 {
		t.Fatalf("expected exactly 1 own image after RunOnce, got %d: %v", len(own), state.images)
	}
	if own[0].ID != "new-id-test-source" {
		t.Errorf("expected the newly uploaded image to remain, got %q", own[0].ID)
	}
	if len(state.deletedIDs) != 1 || state.deletedIDs[0] != "sched-old-1" {
		t.Errorf("expected the old image sched-old-1 to be pruned, got deletedIDs=%v", state.deletedIDs)
	}
}

func TestRunOnce_FailedUploadReturnsErrorAndDoesNotPrune(t *testing.T) {
	// If background processing fails, RunOnce must surface an error and not prune
	// (there is no new image to prune around; the next run reconciles).
	initialImages := []apiImageItem{
		{ID: "sched-old-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	state.failUpload = true
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "test-source",
		Source:         &staticSource{name: "test-source", data: minimalPNG()},
	}

	err := RunOnce(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when upload processing fails, got nil")
	}
	if len(state.deletedIDs) != 0 {
		t.Errorf("expected no deletions on failed upload, got %v", state.deletedIDs)
	}
}

func TestRunOnce_Group_EvictsPeers(t *testing.T) {
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
		Group:          "daily-wallpaper",
		GroupMembers:   []string{"xkcd", "pusheen", "oatmeal"},
		Source:         &staticSource{name: "xkcd", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "xkcd" {
		t.Errorf("expected upload from xkcd, got %q", state.uploadedSource)
	}
	// peer-img-1 (pusheen) and peer-img-2 (oatmeal) must be evicted; own-img-1 pruned (keep 1).
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
		t.Error("expected own-img-1 to be pruned")
	}
}

func TestRunOnce_Group_DoesNotEvictNonMembers(t *testing.T) {
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
		Group:          "daily-wallpaper",
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

func TestRunOnce_NoGroup_DoesNotEvictPeers(t *testing.T) {
	// Without a group, images from other sources are left alone.
	initialImages := []apiImageItem{
		{ID: "other-img", Source: "pusheen"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL: srv.URL,
		SourceName:     "xkcd",
		Source:         &staticSource{name: "xkcd", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	for _, id := range state.deletedIDs {
		if id == "other-img" {
			t.Error("other-img must not be deleted when no group is set")
		}
	}
}

func TestRunOnce_Yield_DeletesOwnAndSkipsUpload(t *testing.T) {
	// External images present + yield → delete own images, don't upload.
	initialImages := []apiImageItem{
		{ID: "external-1", Source: "manual-upload"},
		{ID: "own-1", Source: "test-source"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:   srv.URL,
		SourceName:       "test-source",
		OnExternalImages: OnExternalImagesYield,
		Source:           &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "" {
		t.Errorf("expected no upload on yield, got source %q", state.uploadedSource)
	}
	if len(state.deletedIDs) != 1 || state.deletedIDs[0] != "own-1" {
		t.Errorf("expected own-1 to be deleted on yield, got deletedIDs=%v", state.deletedIDs)
	}
}

func TestRunOnce_Takeover_DeletesExternalImages(t *testing.T) {
	// External images present + takeover → upload and delete all external images.
	initialImages := []apiImageItem{
		{ID: "external-1", Source: "manual-upload"},
		{ID: "external-2", Source: "unknown"},
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:   srv.URL,
		SourceName:       "test-source",
		OnExternalImages: OnExternalImagesTakeover,
		Source:           &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "test-source" {
		t.Errorf("expected upload on takeover, got %q", state.uploadedSource)
	}
	deletedSet := make(map[string]bool, len(state.deletedIDs))
	for _, id := range state.deletedIDs {
		deletedSet[id] = true
	}
	if !deletedSet["external-1"] {
		t.Error("expected external-1 to be deleted on takeover")
	}
	if !deletedSet["external-2"] {
		t.Error("expected external-2 to be deleted on takeover")
	}
}

func TestRunOnce_Yield_NoExternalImages_UploadsNormally(t *testing.T) {
	// No external images; yield should not trigger — upload normally.
	srv, state := newGoframeTestServer(nil)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:   srv.URL,
		SourceName:       "test-source",
		OnExternalImages: OnExternalImagesYield,
		Source:           &staticSource{name: "test-source", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "test-source" {
		t.Errorf("expected upload when no external images, got %q", state.uploadedSource)
	}
}

func TestRunOnce_Yield_GroupMembersNotExternal(t *testing.T) {
	// A group member image must not trigger yield.
	initialImages := []apiImageItem{
		{ID: "peer-img", Source: "pusheen"}, // group member, not external
	}
	srv, state := newGoframeTestServer(initialImages)
	defer srv.Close()

	cfg := Config{
		GoframeBaseURL:   srv.URL,
		SourceName:       "xkcd",
		Group:            "daily-wallpaper",
		GroupMembers:     []string{"xkcd", "pusheen"},
		OnExternalImages: OnExternalImagesYield,
		Source:           &staticSource{name: "xkcd", data: minimalPNG()},
	}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}

	if state.uploadedSource != "xkcd" {
		t.Errorf("expected upload despite peer image (group member should not count as external), got %q", state.uploadedSource)
	}
}

// --- Helper function tests ---

func TestHasExternalImages(t *testing.T) {
	images := []apiImageItem{
		{ID: "1", Source: "xkcd"},
		{ID: "2", Source: "pusheen"},
		{ID: "3", Source: "manual"},
		{ID: "4", Source: ""},
	}
	if !hasExternalImages(images, "xkcd", []string{"xkcd", "pusheen"}) {
		t.Error("expected external images (manual + empty), got false")
	}
	if hasExternalImages(images[:2], "xkcd", []string{"xkcd", "pusheen"}) {
		t.Error("expected no external images with only group members, got true")
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
