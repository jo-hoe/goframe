package nasaapod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestSource constructs a NASAAPODSource pointed at the given test server.
func newTestSource(srv *httptest.Server, apiKey string) *NASAAPODSource {
	return &NASAAPODSource{
		apiKey:     apiKey,
		httpClient: srv.Client(),
		apiURL:     srv.URL + "/apod",
	}
}

func TestParseRandomResponse_Valid(t *testing.T) {
	entries := []apodEntry{
		{Date: "2024-01-15", Title: "Galaxy", MediaType: "image", URL: "https://example.com/img.jpg", HDUrl: "https://example.com/img_hd.jpg"},
	}
	data, _ := json.Marshal(entries)

	got, err := parseRandomResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Date != "2024-01-15" {
		t.Errorf("expected date 2024-01-15, got %q", got.Date)
	}
	if got.MediaType != "image" {
		t.Errorf("expected media_type image, got %q", got.MediaType)
	}
}

func TestParseRandomResponse_InvalidJSON(t *testing.T) {
	_, err := parseRandomResponse([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseRandomResponse_EmptyArray(t *testing.T) {
	data, _ := json.Marshal([]apodEntry{})
	_, err := parseRandomResponse(data)
	if err == nil {
		t.Fatal("expected error for empty array, got nil")
	}
}

func TestParseRandomResponse_MissingMediaType(t *testing.T) {
	entries := []apodEntry{{Date: "2024-01-15", URL: "https://example.com/img.jpg"}}
	data, _ := json.Marshal(entries)
	_, err := parseRandomResponse(data)
	if err == nil {
		t.Fatal("expected error for missing media_type, got nil")
	}
}

func TestApodEntry_BestImageURL_HDPresent(t *testing.T) {
	e := apodEntry{URL: "https://example.com/std.jpg", HDUrl: "https://example.com/hd.jpg"}
	if got := e.bestImageURL(); got != "https://example.com/hd.jpg" {
		t.Errorf("expected HD URL, got %q", got)
	}
}

func TestApodEntry_BestImageURL_HDAbsent(t *testing.T) {
	e := apodEntry{URL: "https://example.com/std.jpg"}
	if got := e.bestImageURL(); got != "https://example.com/std.jpg" {
		t.Errorf("expected standard URL, got %q", got)
	}
}

func TestBuildRandomURL_ContainsCountAndKey(t *testing.T) {
	src := &NASAAPODSource{apiKey: "TESTKEY123", apiURL: "https://api.nasa.gov/planetary/apod"}
	u := src.buildRandomURL()
	for _, want := range []string{"count=1", "api_key=TESTKEY123"} {
		if !contains(u, want) {
			t.Errorf("expected URL to contain %q, got %q", want, u)
		}
	}
}

func TestNewNASAAPODSource_EmptyKeyUsesDemoKey(t *testing.T) {
	src := NewNASAAPODSource("")
	if src.apiKey != demoAPIKey {
		t.Errorf("expected demo key %q, got %q", demoAPIKey, src.apiKey)
	}
}

func TestNewNASAAPODSource_CustomKey(t *testing.T) {
	src := NewNASAAPODSource("MYKEY")
	if src.apiKey != "MYKEY" {
		t.Errorf("expected MYKEY, got %q", src.apiKey)
	}
}

func TestName(t *testing.T) {
	src := NewNASAAPODSource("")
	if src.Name() != "nasaapod" {
		t.Errorf("expected name nasaapod, got %q", src.Name())
	}
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-apod-image")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apod":
			entries := []apodEntry{{
				Date:      "2024-06-01",
				MediaType: "image",
				URL:       fmt.Sprintf("http://%s/img.jpg", r.Host),
				HDUrl:     fmt.Sprintf("http://%s/img_hd.jpg", r.Host),
			}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entries)
		case "/img_hd.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := newTestSource(srv, "TESTKEY")
	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func TestFetch_FallsBackToStandardURL(t *testing.T) {
	imageBytes := []byte("standard-image")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apod":
			entries := []apodEntry{{
				Date:      "2024-06-01",
				MediaType: "image",
				URL:       fmt.Sprintf("http://%s/img_std.jpg", r.Host),
				// HDUrl intentionally absent
			}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entries)
		case "/img_std.jpg":
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := newTestSource(srv, "TESTKEY")
	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func TestFetch_VideoEntryReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entries := []apodEntry{{
			Date:      "2024-06-02",
			MediaType: "video",
			URL:       "https://www.youtube.com/embed/VIDEO_ID",
		}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	src := newTestSource(srv, "TESTKEY")
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for video entry, got nil")
	}
}

func TestFetch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := newTestSource(srv, "TESTKEY")
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

func TestFetch_ImageDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apod":
			entries := []apodEntry{{
				Date:      "2024-06-01",
				MediaType: "image",
				URL:       fmt.Sprintf("http://%s/missing.jpg", r.Host),
			}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entries)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := newTestSource(srv, "TESTKEY")
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for image download failure, got nil")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
