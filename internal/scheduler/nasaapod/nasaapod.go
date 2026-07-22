// Package nasaapod provides an ImageSource that fetches a random image from the
// NASA Astronomy Picture of the Day (APOD) archive via the public NASA APOD API.
//
// API documentation: https://api.nasa.gov/
// Archive viewer:    https://apod.nasa.gov/apod/archivepixFull.html
package nasaapod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const (
	defaultAPIURL = "https://api.nasa.gov/planetary/apod"

	// demoAPIKey is the NASA-sanctioned key for low-volume testing.
	// Production deployments should supply a real API key from https://api.nasa.gov/.
	demoAPIKey = "DEMO_KEY"

	mediaTypeImage = "image"
)

// NASAAPODSource fetches a random image from the NASA APOD archive.
// When the randomly selected entry is a video the fetch is retried up to
// maxVideoSkips times before returning an error.
type NASAAPODSource struct {
	apiKey     string
	httpClient *http.Client
	apiURL     string
}

// NewNASAAPODSource constructs a NASAAPODSource.
// apiKey is a NASA API key from https://api.nasa.gov/.
// Pass an empty string to fall back to the demo key (30 req/hour/IP).
func NewNASAAPODSource(apiKey string) *NASAAPODSource {
	key := apiKey
	if key == "" {
		key = demoAPIKey
	}
	return &NASAAPODSource{
		apiKey:     key,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiURL:     defaultAPIURL,
	}
}

// Name returns the source identifier used in scheduler configs and image metadata.
func (n *NASAAPODSource) Name() string {
	return "nasaapod"
}

// Fetch retrieves a random APOD image from the archive.
// Returns an error when the API returns only video entries.
func (n *NASAAPODSource) Fetch(ctx context.Context) ([]byte, error) {
	meta, err := n.fetchRandomMeta(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching nasa apod metadata: %w", err)
	}
	if meta.MediaType != mediaTypeImage {
		return nil, fmt.Errorf("nasa apod: random entry %q is %q (not an image), skipping", meta.Date, meta.MediaType)
	}

	imageURL := meta.bestImageURL()
	data, err := scheduler.FetchBytes(ctx, n.httpClient, imageURL)
	if err != nil {
		return nil, fmt.Errorf("downloading nasa apod image from %q: %w", imageURL, err)
	}
	return data, nil
}

// apodEntry holds the fields returned by the NASA APOD API for a single entry.
type apodEntry struct {
	Date      string `json:"date"`
	Title     string `json:"title"`
	MediaType string `json:"media_type"`
	// URL is the standard-definition image URL or the YouTube URL for videos.
	URL string `json:"url"`
	// HDUrl is the full-resolution image URL; absent for video entries.
	HDUrl string `json:"hdurl"`
}

// bestImageURL returns the HD image URL when present, falling back to URL.
func (e apodEntry) bestImageURL() string {
	if e.HDUrl != "" {
		return e.HDUrl
	}
	return e.URL
}

func (n *NASAAPODSource) fetchRandomMeta(ctx context.Context) (apodEntry, error) {
	u := n.buildRandomURL()
	data, err := scheduler.FetchBytes(ctx, n.httpClient, u)
	if err != nil {
		return apodEntry{}, err
	}
	return parseRandomResponse(data)
}

// buildRandomURL constructs the APOD API URL that returns one random entry.
func (n *NASAAPODSource) buildRandomURL() string {
	params := url.Values{}
	params.Set("api_key", n.apiKey)
	params.Set("count", "1")
	return n.apiURL + "?" + params.Encode()
}

// parseRandomResponse decodes the JSON array returned by count=1 requests.
func parseRandomResponse(data []byte) (apodEntry, error) {
	var entries []apodEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return apodEntry{}, fmt.Errorf("parsing nasa apod response: %w", err)
	}
	if len(entries) == 0 {
		return apodEntry{}, fmt.Errorf("nasa apod response contained no entries")
	}
	entry := entries[0]
	if entry.MediaType == "" {
		return apodEntry{}, fmt.Errorf("nasa apod entry missing media_type field")
	}
	return entry, nil
}
