package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
	"time"
)

// apiImageItem mirrors the JSON shape returned by GET /api/images.
type apiImageItem struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	ProcessedURL string    `json:"processedUrl"`
	OriginalURL  string    `json:"originalUrl"`
	Source       string    `json:"source"`
}

// testFixturePath is the relative path from the repo root to the test image.
const testFixturePath = "../../internal/imageprocessing/testdata/peppers.png"

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// requireConfig loads the integration config and skips the test if none is found.
func requireConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg == nil || cfg.ServerURL == "" {
		t.Skip("no serverURL configured — copy local.example.yaml to local.yaml to run integration tests")
	}
	return cfg
}

// uploadTestImage uploads peppers.png and returns the image ID.
func uploadTestImage(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()

	data, err := os.ReadFile(testFixturePath)
	if err != nil {
		t.Fatalf("reading test fixture: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "peppers.png")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/image", body)
	if err != nil {
		t.Fatalf("building upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: expected 201, got %d: %s", resp.StatusCode, b)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding upload response: %v", err)
	}
	if result.ID == "" {
		t.Fatal("upload: response contained empty id")
	}
	return result.ID
}

// listImages returns the current image list from GET /api/images.
func listImages(t *testing.T, client *http.Client, baseURL string) []apiImageItem {
	t.Helper()

	resp, err := client.Get(baseURL + "/api/images")
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("list: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var items []apiImageItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decoding list response: %v", err)
	}
	return items
}

// deleteImage deletes an image by ID and asserts 204.
func deleteImage(t *testing.T, client *http.Client, baseURL, id string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/images/%s", baseURL, id), nil)
	if err != nil {
		t.Fatalf("building delete request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("delete %s: expected 204, got %d: %s", id, resp.StatusCode, b)
	}
}

// moveImage calls POST /htmx/image/:id/move?dir=<dir>.
func moveImage(t *testing.T, client *http.Client, baseURL, id, dir string) {
	t.Helper()

	url := fmt.Sprintf("%s/htmx/image/%s/move?dir=%s", baseURL, id, dir)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("building move request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("move request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("move %s dir=%s: expected 200, got %d: %s", id, dir, resp.StatusCode, b)
	}
}

// containsID returns true if any item in the list has the given ID.
func containsID(items []apiImageItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// orderOf returns the position (0-based) of id in the list, or -1 if not found.
func orderOf(items []apiImageItem, id string) int {
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}
