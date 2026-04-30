package tumblr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleRSS = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Test Blog</title>
    <item>
      <description>&lt;figure&gt;&lt;img src="https://64.media.tumblr.com/abc/s640x960/a.jpg" srcset="https://64.media.tumblr.com/abc/s250x400/a.jpg 250w, https://64.media.tumblr.com/abc/s400x600/a.jpg 400w, https://64.media.tumblr.com/abc/s640x960/a.jpg 640w"&gt;&lt;/figure&gt;</description>
    </item>
    <item>
      <description>&lt;p&gt;no image here&lt;/p&gt;</description>
    </item>
    <item>
      <description>&lt;img src="https://64.media.tumblr.com/def/s640x960/b.png"&gt;</description>
    </item>
  </channel>
</rss>`

func TestParseFeed_ExtractsImageItems(t *testing.T) {
	urls, err := parseFeed([]byte(sampleRSS))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 image URLs, got %d: %v", len(urls), urls)
	}
	// First item has srcset — should pick largest (640w entry).
	if !strings.Contains(urls[0], "s640x960/a.jpg") {
		t.Errorf("expected largest srcset URL, got %q", urls[0])
	}
	// Third item has only src — should fall back to it.
	if !strings.Contains(urls[1], "s640x960/b.png") {
		t.Errorf("expected src fallback URL, got %q", urls[1])
	}
}

func TestParseFeed_InvalidXML(t *testing.T) {
	_, err := parseFeed([]byte("<not valid xml>>>"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestParseFeed_Empty(t *testing.T) {
	empty := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`
	urls, err := parseFeed([]byte(empty))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 0 {
		t.Errorf("expected 0 URLs, got %d", len(urls))
	}
}

func TestBestImageURL_PrefersLargestSrcset(t *testing.T) {
	html := `<img src="https://example.com/s100/a.jpg" srcset="https://example.com/s100/a.jpg 100w, https://example.com/s400/a.jpg 400w">`
	got := bestImageURL(html)
	if got != "https://example.com/s400/a.jpg" {
		t.Errorf("expected largest srcset URL, got %q", got)
	}
}

func TestBestImageURL_FallsBackToSrc(t *testing.T) {
	html := `<img src="https://example.com/image.jpg">`
	got := bestImageURL(html)
	if got != "https://example.com/image.jpg" {
		t.Errorf("expected src fallback, got %q", got)
	}
}

func TestBestImageURL_Empty(t *testing.T) {
	if got := bestImageURL("<p>no image</p>"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFetch_FeedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	source := &TumblrSource{blogs: []string{"test"}, httpClient: srv.Client()}
	_, err := source.fetchFeed(context.Background(), srv.URL+"/rss")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rss":
			// Replace all tumblr image URLs in the sample RSS with test server URLs.
			rss := sampleRSS
			replacements := map[string]string{
				"https://64.media.tumblr.com/abc/s640x960/a.jpg": "http://" + r.Host + "/img/a.jpg",
				"https://64.media.tumblr.com/abc/s250x400/a.jpg": "http://" + r.Host + "/img/a-small.jpg",
				"https://64.media.tumblr.com/abc/s400x600/a.jpg": "http://" + r.Host + "/img/a-med.jpg",
				"https://64.media.tumblr.com/def/s640x960/b.png": "http://" + r.Host + "/img/b.png",
			}
			for old, new := range replacements {
				rss = strings.ReplaceAll(rss, old, new)
			}
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rss))
		case "/img/a.jpg", "/img/b.png":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := &TumblrSource{blogs: []string{"test"}, httpClient: srv.Client()}
	urls, err := source.fetchFeed(context.Background(), srv.URL+"/rss")
	if err != nil {
		t.Fatalf("fetchFeed error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 image URLs, got %d: %v", len(urls), urls)
	}
}
