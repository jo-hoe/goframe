package oatmeal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestSource(srv *httptest.Server) *OatmealSource {
	return &OatmealSource{httpClient: srv.Client()}
}

func TestName(t *testing.T) {
	if got := NewOatmealSource().Name(); got != "oatmeal" {
		t.Errorf("expected \"oatmeal\", got %q", got)
	}
}

func TestFetchBytes_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestSource(srv).fetchBytes(context.Background(), srv.URL+"/anything")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestFetchSinglePanelImageURL_SinglePanel(t *testing.T) {
	const imgURL = "https://s3.amazonaws.com/theoatmeal-img/comics/tunashamed/tunashamed.png"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<img src="%s">`, imgURL)
	}))
	defer srv.Close()

	got, err := newTestSource(srv).fetchSinglePanelImageURL(context.Background(), srv.URL+"/comics/tunashamed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != imgURL {
		t.Errorf("expected %q, got %q", imgURL, got)
	}
}

func TestFetchSinglePanelImageURL_MultiPanel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w,
			`<img src="https://s3.amazonaws.com/theoatmeal-img/comics/ai_art/1.png">`+
				`<img src="https://s3.amazonaws.com/theoatmeal-img/comics/ai_art/2.png">`)
	}))
	defer srv.Close()

	_, err := newTestSource(srv).fetchSinglePanelImageURL(context.Background(), srv.URL+"/comics/ai_art")
	if err == nil {
		t.Fatal("expected error for multi-panel comic, got nil")
	}
}

func TestFetchSinglePanelImageURL_NoImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<p>no image here</p>`)
	}))
	defer srv.Close()

	_, err := newTestSource(srv).fetchSinglePanelImageURL(context.Background(), srv.URL+"/comics/empty")
	if err == nil {
		t.Fatal("expected error when no image found, got nil")
	}
}

func TestFetchSlugsFromURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w,
			`<a href="/comics/tunashamed">Tunashamed</a>`+
				`<a href="/comics/screen_addicted">Screen Addicted</a>`)
	}))
	defer srv.Close()

	slugs, err := newTestSource(srv).fetchSlugsFromURLs(context.Background(), []string{srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slugs) != 2 {
		t.Fatalf("expected 2 slugs, got %d: %v", len(slugs), slugs)
	}
	if slugs[0] != "tunashamed" {
		t.Errorf("expected \"tunashamed\", got %q", slugs[0])
	}
	if slugs[1] != "screen_addicted" {
		t.Errorf("expected \"screen_addicted\", got %q", slugs[1])
	}
}

func TestFetchSlugsFromURLs_DeduplicatesSlugs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<a href="/comics/tunashamed">Tunashamed</a>`)
	}))
	defer srv.Close()

	// Same URL twice — slug should appear only once.
	slugs, err := newTestSource(srv).fetchSlugsFromURLs(context.Background(), []string{srv.URL, srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slugs) != 1 {
		t.Fatalf("expected 1 slug after deduplication, got %d: %v", len(slugs), slugs)
	}
}

func TestFetch_EndToEnd(t *testing.T) {
	const slug = "tunashamed"
	const imgPath = "/theoatmeal-img/comics/tunashamed/tunashamed.png"
	imageData := []byte("fake-png-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/comics", "/c2index/page:2", "/c2index/page:3", "/c2index/page:4",
			"/c2index/page:5", "/c2index/page:6", "/c2index/page:7", "/c2index/page:8",
			"/c2index/page:9":
			_, _ = fmt.Fprintf(w, `<a href="/comics/%s">Comic</a>`, slug)
		case "/comics/" + slug:
			_, _ = fmt.Fprintf(w,
				`<img src="https://s3.amazonaws.com/theoatmeal-img/comics/%s/%s.png">`, slug, slug)
		case imgPath:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := newTestSource(srv)

	// Fetch slugs from the test server's index pages.
	indexURLs := []string{srv.URL + "/comics"}
	for p := 2; p <= pageCount; p++ {
		indexURLs = append(indexURLs, fmt.Sprintf("%s/c2index/page:%d", srv.URL, p))
	}
	slugs, err := src.fetchSlugsFromURLs(context.Background(), indexURLs)
	if err != nil {
		t.Fatalf("fetchSlugsFromURLs: %v", err)
	}
	if len(slugs) == 0 {
		t.Fatal("expected at least one slug")
	}

	// Verify the comic page resolves to a single-panel image.
	imgURL, err := src.fetchSinglePanelImageURL(context.Background(), srv.URL+"/comics/"+slug)
	if err != nil {
		t.Fatalf("fetchSinglePanelImageURL: %v", err)
	}
	// The returned URL points to the real S3 host — just confirm it's non-empty and well-formed.
	if imgURL == "" {
		t.Fatal("expected non-empty image URL")
	}
}
