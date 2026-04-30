package tumblr

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"math/rand/v2"
	"net/http"
	"regexp"
	"time"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const rssSuffix = "/rss"

// srcsetPattern extracts all URLs from an HTML srcset attribute value.
// Each entry is "<url> <descriptor>"; we want the URLs.
var srcsetPattern = regexp.MustCompile(`(https?://[^\s,]+)\s+\d+w`)

// imgSrcPattern extracts the src attribute of an img tag.
var imgSrcPattern = regexp.MustCompile(`<img[^>]+src="(https?://[^"]+)"`)

// TumblrSource fetches a random image from a public Tumblr blog RSS feed.
type TumblrSource struct {
	blog       string
	httpClient *http.Client
}

// NewTumblrSource constructs a TumblrSource for the given blog name (e.g. "nasa").
func NewTumblrSource(blog string) *TumblrSource {
	return &TumblrSource{
		blog:       blog,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the source identifier.
func (t *TumblrSource) Name() string {
	return "tumblr"
}

// Fetch retrieves a random image from the blog's RSS feed.
func (t *TumblrSource) Fetch(ctx context.Context) ([]byte, error) {
	feedURL := "https://" + t.blog + ".tumblr.com" + rssSuffix

	items, err := t.fetchFeed(ctx, feedURL)
	if err != nil {
		return nil, fmt.Errorf("fetching tumblr RSS feed for %q: %w", t.blog, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("tumblr RSS feed for %q returned no image items", t.blog)
	}

	// #nosec G404 -- math/rand is intentional; image selection does not require cryptographic randomness
	imageURL := items[rand.IntN(len(items))]

	data, err := scheduler.FetchBytes(ctx, t.httpClient, imageURL)
	if err != nil {
		return nil, fmt.Errorf("downloading tumblr image %s: %w", imageURL, err)
	}
	return data, nil
}

// rssItem holds the description field from an RSS item.
type rssItem struct {
	Description string `xml:"description"`
}

func (t *TumblrSource) fetchFeed(ctx context.Context, feedURL string) ([]string, error) {
	body, err := scheduler.FetchBytes(ctx, t.httpClient, feedURL)
	if err != nil {
		return nil, err
	}
	return parseFeed(body)
}

func parseFeed(data []byte) ([]string, error) {
	type rss struct {
		Items []rssItem `xml:"channel>item"`
	}

	var feed rss
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parsing tumblr RSS: %w", err)
	}

	var imageURLs []string
	for _, item := range feed.Items {
		decoded := html.UnescapeString(item.Description)
		if url := bestImageURL(decoded); url != "" {
			imageURLs = append(imageURLs, url)
		}
	}
	return imageURLs, nil
}

// bestImageURL returns the highest-resolution image URL from an RSS item's HTML.
// It prefers the last (largest) srcset entry; falls back to the img src attribute.
func bestImageURL(htmlContent string) string {
	if urls := srcsetPattern.FindAllStringSubmatch(htmlContent, -1); len(urls) > 0 {
		return urls[len(urls)-1][1]
	}
	if m := imgSrcPattern.FindStringSubmatch(htmlContent); m != nil {
		return m[1]
	}
	return ""
}
