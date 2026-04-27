package pusheen

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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

	source := newTestSource(srv)
	got, err := source.fetchBytes(context.Background(), srv.URL+"/anything")
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

	source := newTestSource(srv)
	_, err := source.fetchBytes(context.Background(), srv.URL+"/anything")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestExtractImageURL_Success(t *testing.T) {
	// Mirrors the actual wire format: JSON with backslash-escaped quotes in the HTML.
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
			// Return a payload whose GIF URL points back at the test server.
			// We use the real pusheen.com domain in the src so the regex matches,
			// but override imgSrcPattern below to also accept the test server host.
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

	// Fetch the API response and extract the URL.
	body, err := source.fetchBytes(context.Background(), srv.URL+"/wp-json/wp/v2/posts")
	if err != nil {
		t.Fatalf("fetchBytes: %v", err)
	}

	imgURL, err := extractImageURL(body)
	if err != nil {
		t.Fatalf("extractImageURL: %v", err)
	}

	// Redirect the image fetch to the test server.
	imgURL = srv.URL + "/wp-content/uploads/2026/04/test.gif"

	got, err := source.fetchBytes(context.Background(), imgURL)
	if err != nil {
		t.Fatalf("fetchBytes image: %v", err)
	}
	if string(got) != string(imageData) {
		t.Errorf("expected %q, got %q", imageData, got)
	}
}
