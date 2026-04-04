package xkcd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestSource returns an XKCDSource wired to the given test server.
func newTestSource(server *httptest.Server) *XKCDSource {
	return &XKCDSource{httpClient: server.Client()}
}

func TestFetch_Success(t *testing.T) {
	imageData := []byte("fake-image-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info.0.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"num":10,"img":"http://%s/img.png"}`, r.Host) // #nosec G705 -- r.Host is the test server address, not user input
		case "/5/info.0.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"num":5,"img":"http://%s/img.png"}`, r.Host) // #nosec G705 -- r.Host is the test server address, not user input
		case "/img.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := newTestSource(srv)
	// Override URLs to point at the test server.
	got, err := source.fetchComicMeta(context.Background(), srv.URL+"/info.0.json")
	if err != nil {
		t.Fatalf("fetchComicMeta latest: %v", err)
	}
	if got.Num != 10 {
		t.Errorf("expected Num=10, got %d", got.Num)
	}

	comic, err := source.fetchComicMeta(context.Background(), srv.URL+"/5/info.0.json")
	if err != nil {
		t.Fatalf("fetchComicMeta comic: %v", err)
	}

	data, err := source.fetchImageBytes(context.Background(), comic.ImgURL)
	if err != nil {
		t.Fatalf("fetchImageBytes: %v", err)
	}
	if string(data) != string(imageData) {
		t.Errorf("expected %q, got %q", imageData, data)
	}
}

func TestFetch_LatestMetaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	source := newTestSource(srv)
	_, err := source.fetchComicMeta(context.Background(), srv.URL+"/info.0.json")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestFetch_ComicMetaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info.0.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"num":5,"img":""}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	source := newTestSource(srv)
	_, err := source.fetchComicMeta(context.Background(), srv.URL+"/3/info.0.json")
	if err == nil {
		t.Fatal("expected error for comic meta 404, got nil")
	}
}

func TestFetch_ImageDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	source := newTestSource(srv)
	_, err := source.fetchImageBytes(context.Background(), srv.URL+"/img.png")
	if err == nil {
		t.Fatal("expected error for image download failure, got nil")
	}
}

func TestRandomComicNumber_ExcludesComic404(t *testing.T) {
	// Run many iterations to have a high probability of hitting 404 if not excluded.
	for i := 0; i < 10000; i++ {
		n := randomComicNumber(1000)
		if n == comicNum404 {
			t.Fatalf("randomComicNumber returned excluded comic %d", comicNum404)
		}
		if n < 1 || n > 1000 {
			t.Fatalf("randomComicNumber returned out-of-range value %d", n)
		}
	}
}

func TestRandomComicNumber_SingleComic(t *testing.T) {
	// When only one comic exists, always return 1.
	for i := 0; i < 100; i++ {
		if n := randomComicNumber(1); n != 1 {
			t.Fatalf("expected 1, got %d", n)
		}
	}
}

func TestRandomComicNumber_LatestBelowExclusion(t *testing.T) {
	// When latestNum < 404, the exclusion gap is never reached.
	for i := 0; i < 1000; i++ {
		n := randomComicNumber(100)
		if n < 1 || n > 100 {
			t.Fatalf("out-of-range value %d for latestNum=100", n)
		}
	}
}
