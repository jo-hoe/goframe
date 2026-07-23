// Package nasaimageoftheday provides an ImageSource that fetches the most recent
// image from the NASA Image of the Day RSS feed.
//
// Feed URL: https://www.nasa.gov/feed/
// Archive:  https://www.nasa.gov/image-of-the-day/
package nasaimageoftheday

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const defaultFeedURL = "https://www.nasa.gov/feed/"

// NASAImageOfTheDaySource fetches the latest image from the NASA Image of the Day RSS feed.
// The RSS feed does not carry media enclosures; the image URL is embedded as an
// <img src="…"> tag inside the content:encoded element of the first item.
type NASAImageOfTheDaySource struct {
	httpClient *http.Client
	feedURL    string
}

// NewNASAImageOfTheDaySource constructs a NASAImageOfTheDaySource with a default HTTP client.
func NewNASAImageOfTheDaySource() *NASAImageOfTheDaySource {
	return &NASAImageOfTheDaySource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		feedURL:    defaultFeedURL,
	}
}

// Name returns the source identifier used in scheduler configs and image metadata.
func (n *NASAImageOfTheDaySource) Name() string {
	return "nasaimageoftheday"
}

// Fetch retrieves the image from the most recent NASA Image of the Day entry.
func (n *NASAImageOfTheDaySource) Fetch(ctx context.Context) ([]byte, error) {
	feed, err := n.fetchFeed(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching nasa image of the day feed: %w", err)
	}

	imageURL, err := extractLatestImageURL(feed)
	if err != nil {
		return nil, fmt.Errorf("extracting image URL from nasa feed: %w", err)
	}

	imageURL = stripQueryParams(imageURL)

	data, err := scheduler.FetchBytes(ctx, n.httpClient, imageURL)
	if err != nil {
		return nil, fmt.Errorf("downloading nasa image of the day from %q: %w", imageURL, err)
	}
	return data, nil
}

// rssFeed is the top-level RSS document.
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

// rssChannel holds the feed metadata and items.
type rssChannel struct {
	Items []rssItem `xml:"item"`
}

// rssItem represents a single feed entry.
// ContentEncoded is namespace-qualified to match the content:encoded element.
type rssItem struct {
	Link           string `xml:"link"`
	ContentEncoded string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

func (n *NASAImageOfTheDaySource) fetchFeed(ctx context.Context) (rssFeed, error) {
	data, err := scheduler.FetchBytes(ctx, n.httpClient, n.feedURL)
	if err != nil {
		return rssFeed{}, err
	}
	return parseFeed(data)
}

func parseFeed(data []byte) (rssFeed, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return rssFeed{}, fmt.Errorf("parsing rss feed: %w", err)
	}
	return feed, nil
}

// extractLatestImageURL finds the image URL in the most recent image-article RSS item.
// The general NASA feed mixes post types; image-of-the-day entries have links under /image-article/.
func extractLatestImageURL(feed rssFeed) (string, error) {
	if len(feed.Channel.Items) == 0 {
		return "", fmt.Errorf("rss feed contains no items")
	}
	for _, item := range feed.Channel.Items {
		if strings.Contains(item.Link, "/image-article/") {
			return extractImgSrc(item.ContentEncoded)
		}
	}
	return "", fmt.Errorf("no image-article item found in rss feed")
}

// extractImgSrc parses an HTML fragment and returns the src attribute of the first <img> element.
func extractImgSrc(htmlFragment string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlFragment))
	if err != nil {
		return "", fmt.Errorf("parsing content html: %w", err)
	}

	src := findFirstImgSrc(doc)
	if src == "" {
		return "", fmt.Errorf("no <img> element found in feed item content")
	}
	return src, nil
}

// findFirstImgSrc walks the HTML node tree and returns the src of the first img element.
func findFirstImgSrc(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "img" {
		for _, attr := range n.Attr {
			if attr.Key == "src" && attr.Val != "" {
				return attr.Val
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if src := findFirstImgSrc(c); src != "" {
			return src
		}
	}
	return ""
}

// stripQueryParams removes query string parameters from a URL, returning the bare path.
func stripQueryParams(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	return u.String()
}
