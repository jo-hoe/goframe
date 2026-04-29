package deviantart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const sampleFeed = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"
  xmlns:media="http://search.yahoo.com/mrss/"
  xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>DeviantArt RSS</title>
    <item>
      <title>Image One</title>
      <media:content url="http://example.com/image1.jpg" height="600" width="800" medium="image"/>
    </item>
    <item>
      <title>Image Two</title>
      <media:content url="http://example.com/image2.png" height="400" width="600" medium="image"/>
    </item>
    <item>
      <title>Not An Image</title>
      <media:content url="http://example.com/video.mp4" medium="video"/>
    </item>
  </channel>
</rss>`

func TestParseFeed_ExtractsImageItems(t *testing.T) {
	items, err := parseFeed([]byte(sampleFeed))
	if err != nil {
		t.Fatalf("parseFeed error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 image items, got %d", len(items))
	}
	if items[0].URL != "http://example.com/image1.jpg" {
		t.Errorf("unexpected URL[0]: %q", items[0].URL)
	}
	if items[1].URL != "http://example.com/image2.png" {
		t.Errorf("unexpected URL[1]: %q", items[1].URL)
	}
}

func TestParseFeed_Empty(t *testing.T) {
	empty := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`
	items, err := parseFeed([]byte(empty))
	if err != nil {
		t.Fatalf("parseFeed error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseFeed_InvalidXML(t *testing.T) {
	_, err := parseFeed([]byte("<not valid xml>>>"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestFetch_FeedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	source := &DeviantArtSource{query: "tag:test", httpClient: srv.Client()}
	_, err := source.fetchFeed(context.Background(), srv.URL+"/rss.xml")
	if err == nil {
		t.Fatal("expected error for non-200 feed response, got nil")
	}
}

func TestFetch_ImageDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := scheduler.FetchBytes(context.Background(), srv.Client(), srv.URL+"/img.jpg")
	if err == nil {
		t.Fatal("expected error for image download failure, got nil")
	}
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rss.xml"):
			feed := strings.ReplaceAll(sampleFeed,
				"http://example.com/image1.jpg",
				"http://"+r.Host+"/img/1.jpg",
			)
			feed = strings.ReplaceAll(feed,
				"http://example.com/image2.png",
				"http://"+r.Host+"/img/2.png",
			)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(feed)) // #nosec G705 -- feed is a test constant, not user input
		case strings.HasPrefix(r.URL.Path, "/img/"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := &DeviantArtSource{query: "tag:test", httpClient: srv.Client()}

	items, err := source.fetchFeed(context.Background(), srv.URL+"/rss.xml?type=deviation&q=tag%3Atest")
	if err != nil {
		t.Fatalf("fetchFeed error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	data, err := scheduler.FetchBytes(context.Background(), source.httpClient, items[0].URL)
	if err != nil {
		t.Fatalf("FetchBytes error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}
