package pusheen

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

func newTestSource(srv *httptest.Server) *PusheenSource {
	return &PusheenSource{httpClient: srv.Client()}
}

func TestName(t *testing.T) {
	if got := NewPusheenSource().Name(); got != "pusheen" {
		t.Errorf("expected \"pusheen\", got %q", got)
	}
}

func TestFetchBytes_Success(t *testing.T) {
	data := []byte("response-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	got, err := scheduler.FetchBytes(context.Background(), srv.Client(), srv.URL+"/anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("expected %q, got %q", data, got)
	}
}

func TestFetchBytes_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := scheduler.FetchBytes(context.Background(), srv.Client(), srv.URL+"/anything")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestExtractImageURL_Success(t *testing.T) {
	body := []byte(`[{"content":{"rendered":"<img src=\"https://pusheen.com/wp-content/uploads/2026/04/Meowdy.gif\" />"}}]`)
	got, err := extractImageURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://pusheen.com/wp-content/uploads/2026/04/Meowdy.gif"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestExtractImageURL_EscapedSlashes(t *testing.T) {
	body := []byte(`[{"content":{"rendered":"<img width=\"1080\" height=\"1080\" src=\"https:\/\/pusheen.com\/wp-content\/uploads\/2026\/04\/Meowdy.gif\" \/>"}}]`)
	got, err := extractImageURL(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://pusheen.com/wp-content/uploads/2026/04/Meowdy.gif"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestExtractImageURL_NoMatch(t *testing.T) {
	body := []byte(`[{"content":{"rendered":"<p>no image here<\/p>"}}]`)
	_, err := extractImageURL(body)
	if err == nil {
		t.Fatal("expected error when no GIF src found, got nil")
	}
}

func TestExtractImageURL_IgnoresNonGIF(t *testing.T) {
	body := []byte(`[{"content":{"rendered":"<img src=\"https://pusheen.com/wp-content/uploads/2026/04/photo.png\" />"}}]`)
	_, err := extractImageURL(body)
	if err == nil {
		t.Fatal("expected error for non-GIF image, got nil")
	}
}

func TestExtractImageURL_IgnoresExternalDomain(t *testing.T) {
	body := []byte(`[{"content":{"rendered":"<img src=\"https://external.com/wp-content/uploads/2026/04/img.gif\" />"}}]`)
	_, err := extractImageURL(body)
	if err == nil {
		t.Fatal("expected error for non-pusheen.com domain, got nil")
	}
}

func TestFetch_EndToEnd(t *testing.T) {
	imageData := []byte("fake-gif-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wp-json/wp/v2/posts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w,
				`[{"content":{"rendered":"<img src=\"https://pusheen.com/wp-content/uploads/2026/04/test.gif\" />"}}]`)
		case "/wp-content/uploads/2026/04/test.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write(imageData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := newTestSource(srv)

	body, err := scheduler.FetchBytes(context.Background(), source.httpClient, srv.URL+"/wp-json/wp/v2/posts")
	if err != nil {
		t.Fatalf("FetchBytes: %v", err)
	}
	if _, err = extractImageURL(body); err != nil {
		t.Fatalf("extractImageURL: %v", err)
	}

	imgURL := srv.URL + "/wp-content/uploads/2026/04/test.gif"
	got, err := scheduler.FetchBytes(context.Background(), source.httpClient, imgURL)
	if err != nil {
		t.Fatalf("FetchBytes image: %v", err)
	}
	if string(got) != string(imageData) {
		t.Errorf("expected %q, got %q", imageData, got)
	}
}
