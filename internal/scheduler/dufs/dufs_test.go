package dufs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// listResponse builds a dufs JSON listing body from a set of entries.
func listResponse(t *testing.T, entries []pathItem) string {
	t.Helper()
	data, err := json.Marshal(indexData{Paths: entries})
	if err != nil {
		t.Fatalf("marshalling listing: %v", err)
	}
	return string(data)
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Has("json"):
			_, _ = w.Write([]byte(listResponse(t, []pathItem{
				{PathType: "File", Name: "cat.jpg"},
				{PathType: "Dir", Name: "subdir"},
				{PathType: "File", Name: "notes.txt"}, // non-image, filtered out
			})))
		case r.URL.Path == "/cat.jpg":
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := NewDufsSource(Config{Endpoint: srv.URL})
	data, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func TestFetch_BasicAuth(t *testing.T) {
	const user, pass = "admin", "s3cr3t"
	var sawAuth bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if ok && u == user && p == pass {
			sawAuth = true
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Query().Has("json"):
			_, _ = w.Write([]byte(listResponse(t, []pathItem{{PathType: "File", Name: "a.png"}})))
		case r.URL.Path == "/a.png":
			_, _ = w.Write([]byte("png-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := NewDufsSource(Config{Endpoint: srv.URL, Username: user, Password: pass})
	if _, err := source.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if !sawAuth {
		t.Error("expected basic auth header to be sent")
	}
}

func TestFetch_NoImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(listResponse(t, []pathItem{
			{PathType: "File", Name: "readme.md"},
			{PathType: "Dir", Name: "photos"},
		})))
	}))
	defer srv.Close()

	source := NewDufsSource(Config{Endpoint: srv.URL})
	_, err := source.Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no images found") {
		t.Errorf("expected no-images error, got %v", err)
	}
}

func TestFetch_TrimsTrailingSlash(t *testing.T) {
	var listPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("json") {
			listPath = r.URL.Path
			_, _ = w.Write([]byte(listResponse(t, []pathItem{{PathType: "File", Name: "x.gif"}})))
			return
		}
		_, _ = w.Write([]byte("gif"))
	}))
	defer srv.Close()

	source := NewDufsSource(Config{Endpoint: srv.URL + "/"})
	if _, err := source.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if listPath != "/" {
		t.Errorf("list path = %q, want /", listPath)
	}
}

func TestIsImageFile(t *testing.T) {
	cases := []struct {
		item pathItem
		want bool
	}{
		{pathItem{PathType: "File", Name: "a.JPG"}, true},
		{pathItem{PathType: "SymlinkFile", Name: "b.png"}, true},
		{pathItem{PathType: "File", Name: "c.txt"}, false},
		{pathItem{PathType: "Dir", Name: "d.jpg"}, false},
		{pathItem{PathType: "File", Name: "noext"}, false},
	}
	for _, tc := range cases {
		if got := isImageFile(tc.item); got != tc.want {
			t.Errorf("isImageFile(%+v) = %v, want %v", tc.item, got, tc.want)
		}
	}
}

func TestName(t *testing.T) {
	if got := NewDufsSource(Config{}).Name(); got != "dufs" {
		t.Errorf("Name() = %q, want dufs", got)
	}
}
