package deviantart

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const rssURL = "https://backend.deviantart.com/rss.xml"

// DeviantArtSource fetches a random image matching a configurable query from the DeviantArt RSS feed.
type DeviantArtSource struct {
	query      string
	httpClient *http.Client
}

// NewDeviantArtSource constructs a DeviantArtSource for the given RSS query string.
// The query uses DeviantArt search syntax, e.g. "boost:popular tag:lofi".
func NewDeviantArtSource(query string) *DeviantArtSource {
	return &DeviantArtSource{
		query:      query,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the source identifier.
func (d *DeviantArtSource) Name() string {
	return "deviantart"
}

// Fetch retrieves a random image from the DeviantArt RSS feed matching the configured query.
func (d *DeviantArtSource) Fetch(ctx context.Context) ([]byte, error) {
	feedURL := rssURL + "?type=deviation&q=" + url.QueryEscape(d.query)

	items, err := d.fetchFeed(ctx, feedURL)
	if err != nil {
		return nil, fmt.Errorf("fetching deviantart RSS feed (query %q): %w", d.query, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("deviantart RSS feed returned no images for query %q", d.query)
	}

	// #nosec G404 -- math/rand is intentional; image selection does not require cryptographic randomness
	item := items[rand.IntN(len(items))]

	data, err := scheduler.FetchBytes(ctx, d.httpClient, item.URL)
	if err != nil {
		return nil, fmt.Errorf("downloading deviantart image %s: %w", item.URL, err)
	}
	return data, nil
}

// mediaItem holds the URL of a single image from the RSS feed.
type mediaItem struct {
	URL string
}

func (d *DeviantArtSource) fetchFeed(ctx context.Context, feedURL string) ([]mediaItem, error) {
	body, err := scheduler.FetchBytes(ctx, d.httpClient, feedURL)
	if err != nil {
		return nil, err
	}
	return parseFeed(body)
}

func parseFeed(data []byte) ([]mediaItem, error) {
	// The feed uses a custom namespace for media:content. Go's xml package matches
	// elements by local name when the namespace prefix is declared on the root element,
	// so we decode with a namespace-aware decoder and match on the local name.
	decoder := xml.NewDecoder(bytes.NewReader(data))

	var items []mediaItem
	var inItem bool

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing RSS XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "item" {
				inItem = true
			}
			if inItem && t.Name.Local == "content" {
				if item, ok := mediaItemFromAttrs(t.Attr); ok {
					items = append(items, item)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "item" {
				inItem = false
			}
		}
	}

	return items, nil
}

func mediaItemFromAttrs(attrs []xml.Attr) (mediaItem, bool) {
	var imgURL, medium string
	for _, attr := range attrs {
		switch attr.Name.Local {
		case "url":
			imgURL = attr.Value
		case "medium":
			medium = attr.Value
		}
	}
	if medium != "image" || imgURL == "" {
		return mediaItem{}, false
	}
	return mediaItem{URL: imgURL}, true
}
