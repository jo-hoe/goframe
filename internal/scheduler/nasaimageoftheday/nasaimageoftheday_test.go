package nasaimageoftheday

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestSource constructs a NASAImageOfTheDaySource pointed at the given test server.
func newTestSource(srv *httptest.Server) *NASAImageOfTheDaySource {
	return &NASAImageOfTheDaySource{
		httpClient: srv.Client(),
		feedURL:    srv.URL + "/feed",
	}
}

func TestParseFeed_Valid(t *testing.T) {
	feed := rssFeed{
		Channel: rssChannel{
			Items: []rssItem{
				{ContentEncoded: `<img src="https://example.com/photo.jpg" />`},
			},
		},
	}
	data, _ := xml.Marshal(feed)
	// xml.Marshal doesn't add the RSS wrapper; test parseFeed via a hand-crafted document.
	raw := buildTestFeedXML(`<img src="https://example.com/photo.jpg" />`)
	got, err := parseFeed([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Channel.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got.Channel.Items))
	}
	_ = data // suppress unused warning
}

func TestParseFeed_InvalidXML(t *testing.T) {
	_, err := parseFeed([]byte("not xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestExtractImgSrc_Found(t *testing.T) {
	html := `<div><p>Some text</p><img src="https://example.com/img.jpg" alt="photo"/></div>`
	got, err := extractImgSrc(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/img.jpg" {
		t.Errorf("expected image URL, got %q", got)
	}
}

func TestExtractImgSrc_NotFound(t *testing.T) {
	_, err := extractImgSrc(`<div><p>No image here</p></div>`)
	if err == nil {
		t.Fatal("expected error when no img element present, got nil")
	}
}

func TestExtractImgSrc_ImgWithoutSrc(t *testing.T) {
	_, err := extractImgSrc(`<img alt="no src attr" />`)
	if err == nil {
		t.Fatal("expected error for img with no src attribute, got nil")
	}
}

func TestExtractLatestImageURL_NoItems(t *testing.T) {
	_, err := extractLatestImageURL(rssFeed{})
	if err == nil {
		t.Fatal("expected error for empty feed, got nil")
	}
}

func TestExtractLatestImageURL_UsesImageArticleItem(t *testing.T) {
	feed := rssFeed{Channel: rssChannel{Items: []rssItem{
		{Link: "https://www.nasa.gov/news/some-news/", ContentEncoded: `<img src="https://example.com/news.jpg"/>`},
		{Link: "https://www.nasa.gov/image-article/galaxy/", ContentEncoded: `<img src="https://example.com/galaxy.jpg"/>`},
	}}}
	got, err := extractLatestImageURL(feed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/galaxy.jpg" {
		t.Errorf("expected image-article URL, got %q", got)
	}
}

func TestExtractLatestImageURL_NoImageArticleItem(t *testing.T) {
	feed := rssFeed{Channel: rssChannel{Items: []rssItem{
		{Link: "https://www.nasa.gov/news/some-news/", ContentEncoded: `<img src="https://example.com/news.jpg"/>`},
	}}}
	_, err := extractLatestImageURL(feed)
	if err == nil {
		t.Fatal("expected error when no image-article item found, got nil")
	}
}

func TestStripQueryParams(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/img.jpg?w=864", "https://example.com/img.jpg"},
		{"https://example.com/img.jpg?w=2048&h=1365", "https://example.com/img.jpg"},
		{"https://example.com/img.jpg", "https://example.com/img.jpg"},
		{"not-a-url", "not-a-url"},
	}
	for _, tc := range tests {
		got := stripQueryParams(tc.input)
		if got != tc.want {
			t.Errorf("stripQueryParams(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestName(t *testing.T) {
	src := NewNASAImageOfTheDaySource()
	if src.Name() != "nasaimageoftheday" {
		t.Errorf("expected name nasaimageoftheday, got %q", src.Name())
	}
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-nasa-image")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			raw := buildTestFeedXML(`<img src="http://` + r.Host + `/photo.jpg" />`)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(raw))
		case "/photo.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := newTestSource(srv)
	data, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func TestFetch_FeedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := newTestSource(srv)
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for feed fetch failure, got nil")
	}
}

func TestFetch_NoImageInFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := buildTestFeedXML(`<p>No image here</p>`)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(raw))
	}))
	defer srv.Close()

	src := newTestSource(srv)
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error when feed item has no image, got nil")
	}
}

func TestFetch_ImageDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			raw := buildTestFeedXML(`<img src="http://` + r.Host + `/missing.jpg" />`)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(raw))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := newTestSource(srv)
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for image download failure, got nil")
	}
}

// buildTestFeedXML returns a minimal RSS 2.0 document with content:encoded set to body.
func buildTestFeedXML(contentEncodedBody string) string {
	return strings.Join([]string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">`,
		`  <channel>`,
		`    <title>NASA</title>`,
		`    <item>`,
		`      <title>Test Image</title>`,
		`      <link>https://www.nasa.gov/image-article/test/</link>`,
		`      <content:encoded><![CDATA[` + contentEncodedBody + `]]></content:encoded>`,
		`    </item>`,
		`  </channel>`,
		`</rss>`,
	}, "\n")
}
