// Package imagelist provides an ImageSource that picks a random image URL from a
// YAML list file and downloads it.
//
// The list file is a YAML sequence where each entry has a required "url" field.
// Any additional fields (title, description, credit, altText, publishedAt,
// width, height, orientation, ...) are ignored. Example:
//
//	# generatedAt: 2026-07-25T17:44:44Z
//	# source: wayback:-
//	- url: https://example.com/a.jpg
//	- url: https://example.com/b.jpg
//	  title: Behind the Scenes
//	- url: https://example.com/c.jpg
//	  title: Adirondack High
//	  description: Yellow plants grow near a rocky stream...
//
// The file is typically mounted from a Kubernetes ConfigMap; its path is passed
// to NewImageListSource by the scheduler binary.
package imagelist

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"github.com/jo-hoe/goframe/internal/scheduler"
	"gopkg.in/yaml.v3"
)

// ImageListSource picks a random image URL from a YAML list file and downloads it.
type ImageListSource struct {
	listPath   string
	httpClient *http.Client
	headers    map[string]string
}

// NewImageListSource constructs an ImageListSource that reads the URL list from listPath.
// headers are applied to each image download request; pass nil for the default request
// headers. This lets the list target hosts that reject the default Go User-Agent.
func NewImageListSource(listPath string, headers map[string]string) *ImageListSource {
	return &ImageListSource{
		listPath:   listPath,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		headers:    headers,
	}
}

// Name returns the source identifier used in scheduler configs and image metadata.
func (s *ImageListSource) Name() string {
	return "imagelist"
}

// Fetch reads the URL list, picks one entry at random, and downloads its image bytes.
func (s *ImageListSource) Fetch(ctx context.Context) ([]byte, error) {
	urls, err := s.loadURLs()
	if err != nil {
		return nil, err
	}

	// #nosec G404 -- math/rand is intentional; image selection does not require cryptographic randomness
	imageURL := urls[rand.IntN(len(urls))]
	slog.Info("imagelist: selected image", "url", imageURL)

	data, err := scheduler.FetchBytesWithHeaders(ctx, s.httpClient, imageURL, s.headers)
	if err != nil {
		return nil, fmt.Errorf("downloading imagelist image from %q: %w", imageURL, err)
	}
	return data, nil
}

// loadURLs reads and parses the list file, returning the usable image URLs.
func (s *ImageListSource) loadURLs() ([]string, error) {
	// #nosec G304 -- reading the list from a configured path is intended
	data, err := os.ReadFile(s.listPath)
	if err != nil {
		return nil, fmt.Errorf("reading imagelist file %q: %w", s.listPath, err)
	}
	return parseImageList(data)
}

// imageEntry holds the single field we consume from each list entry.
// Additional YAML fields are ignored.
type imageEntry struct {
	URL string `yaml:"url"`
}

// parseImageList decodes the YAML list and collects non-empty URLs.
// It returns an error when the list yields no usable URLs.
func parseImageList(data []byte) ([]string, error) {
	var entries []imageEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing imagelist: %w", err)
	}

	urls := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.URL != "" {
			urls = append(urls, e.URL)
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("imagelist contains no image URLs")
	}
	return urls, nil
}
