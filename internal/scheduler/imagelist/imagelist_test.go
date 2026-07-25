package imagelist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeListFile writes content to a temp file and returns its path.
func writeListFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "images.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing list file: %v", err)
	}
	return path
}

func TestName(t *testing.T) {
	src := NewImageListSource("/tmp/images.yaml", nil)
	if src.Name() != "imagelist" {
		t.Errorf("expected name imagelist, got %q", src.Name())
	}
}

func TestParseImageList_Valid(t *testing.T) {
	data := []byte(`# generatedAt: 2026-07-25T17:44:44Z
# source: wayback:-
- url: https://example.com/a.jpg
- url: https://example.com/b.jpg
  title: Behind the Scenes
- url: https://example.com/c.jpg
  title: Adirondack High
  description: Yellow plants grow near a rocky stream.
  credit: Someone
  width: 1900
  height: 997
`)
	urls, err := parseImageList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		"https://example.com/a.jpg",
		"https://example.com/b.jpg",
		"https://example.com/c.jpg",
	}
	if len(urls) != len(want) {
		t.Fatalf("expected %d urls, got %d (%v)", len(want), len(urls), urls)
	}
	for i, u := range want {
		if urls[i] != u {
			t.Errorf("url[%d]: expected %q, got %q", i, u, urls[i])
		}
	}
}

func TestParseImageList_SkipsEntriesWithoutURL(t *testing.T) {
	data := []byte(`- url: https://example.com/a.jpg
- title: no url here
- url: ""
- url: https://example.com/b.jpg
`)
	urls, err := parseImageList(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"https://example.com/a.jpg", "https://example.com/b.jpg"}
	if len(urls) != len(want) {
		t.Fatalf("expected %d urls, got %d (%v)", len(want), len(urls), urls)
	}
}

func TestParseImageList_SingleEntry(t *testing.T) {
	urls, err := parseImageList([]byte("- url: https://example.com/only.jpg\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://example.com/only.jpg" {
		t.Errorf("expected single url, got %v", urls)
	}
}

func TestParseImageList_EmptyOrCommentsOnly(t *testing.T) {
	for _, content := range []string{
		"",
		"# generatedAt: 2026-07-25T17:44:44Z\n# source: wayback:-\n",
		"[]\n",
	} {
		if _, err := parseImageList([]byte(content)); err == nil {
			t.Errorf("expected error for %q, got nil", content)
		}
	}
}

func TestParseImageList_InvalidYAML(t *testing.T) {
	if _, err := parseImageList([]byte("this: [is: not: valid")); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// newTestSource builds a source pointed at listPath with the test server's client.
func newTestSource(listPath string, client *http.Client) *ImageListSource {
	return &ImageListSource{listPath: listPath, httpClient: client}
}

func TestFetch_SendsConfiguredHeaders(t *testing.T) {
	imageBytes := []byte("hdr-image")
	var gotUA, gotCustom string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/only.jpg" {
			gotUA = r.Header.Get("User-Agent")
			gotCustom = r.Header.Get("X-Custom")
			_, _ = w.Write(imageBytes)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	path := writeListFile(t, "- url: "+srv.URL+"/only.jpg\n")
	src := &ImageListSource{
		listPath:   path,
		httpClient: srv.Client(),
		headers:    map[string]string{"User-Agent": "Mozilla/5.0 (test)", "X-Custom": "abc"},
	}

	if _, err := src.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if gotUA != "Mozilla/5.0 (test)" {
		t.Errorf("expected configured User-Agent, got %q", gotUA)
	}
	if gotCustom != "abc" {
		t.Errorf("expected X-Custom header, got %q", gotCustom)
	}
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-image-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/only.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Single entry so selection is deterministic.
	path := writeListFile(t, "- url: "+srv.URL+"/only.jpg\n")
	src := newTestSource(path, srv.Client())

	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func TestFetch_MissingFile(t *testing.T) {
	src := newTestSource(filepath.Join(t.TempDir(), "does-not-exist.yaml"), &http.Client{Timeout: time.Second})
	if _, err := src.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestFetch_EmptyList(t *testing.T) {
	path := writeListFile(t, "# only comments\n")
	src := newTestSource(path, &http.Client{Timeout: time.Second})
	if _, err := src.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for empty list, got nil")
	}
}

func TestFetch_DownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	path := writeListFile(t, "- url: "+srv.URL+"/missing.jpg\n")
	src := newTestSource(path, srv.Client())
	if _, err := src.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for download failure, got nil")
	}
}
