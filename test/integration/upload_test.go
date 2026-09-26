package integration

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
)

// TestUpload verifies the happy-path async flow: submit returns 202 + id, and
// the derived status eventually reaches "succeeded" with the image listed.
func TestUpload(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	id := uploadTestImage(t, client, cfg.ServerURL)
	t.Logf("uploaded image id: %s", id)
	t.Cleanup(func() { deleteImage(t, client, cfg.ServerURL, id) })

	if !containsID(listImages(t, client, cfg.ServerURL), id) {
		t.Fatalf("uploaded image %s not present in list", id)
	}
}

// TestUploadIdempotent verifies that re-uploading identical bytes yields the
// same content-addressed id and does not create a duplicate list entry.
func TestUploadIdempotent(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	data, err := os.ReadFile(testFixturePath)
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}

	firstID := submitImage(t, client, cfg.ServerURL, data)
	waitForStatus(t, client, cfg.ServerURL, firstID, "succeeded")
	t.Cleanup(func() { deleteImage(t, client, cfg.ServerURL, firstID) })

	secondID := submitImage(t, client, cfg.ServerURL, data)
	if secondID != firstID {
		t.Fatalf("re-upload produced different id: first=%s second=%s", firstID, secondID)
	}

	// The re-upload must not create a duplicate list entry.
	count := 0
	for _, item := range listImages(t, client, cfg.ServerURL) {
		if item.ID == firstID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one list entry for %s, got %d", firstID, count)
	}
}

// TestUploadGarbageRejected verifies that lightweight synchronous validation
// rejects non-image bytes with 400 before accepting for processing.
func TestUploadGarbageRejected(t *testing.T) {
	cfg := requireConfig(t)
	client := newHTTPClient()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "garbage.png")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write([]byte("this is definitely not an image")); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.ServerURL+"/api/image", body)
	if err != nil {
		t.Fatalf("building upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("garbage upload: expected 400, got %d: %s", resp.StatusCode, b)
	}
}
