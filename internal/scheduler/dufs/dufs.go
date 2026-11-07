// Package dufs implements an image source that pulls a random image from a dufs
// (https://github.com/sigoden/dufs) static file server — typically the image
// store app bundled with goframe, but any dufs instance works.
package dufs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Config holds the parameters needed to connect to a dufs instance.
type Config struct {
	// Endpoint is the base URL of the dufs instance, including any path prefix
	// (e.g. "http://myrelease-imagestore:5000/gallery"). No trailing slash.
	Endpoint string
	// Username is the HTTP basic-auth user. Empty means anonymous access.
	Username string
	// Password is the HTTP basic-auth password.
	Password string
}

// imageExtensions is the set of file extensions treated as images when filtering
// the dufs directory listing. Kept lowercase; compared case-insensitively.
var imageExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {},
	".webp": {}, ".bmp": {}, ".tiff": {}, ".svg": {},
}

// DufsSource fetches a random image from a dufs file server. It lists the served
// directory via the dufs JSON listing, filters to image files, and downloads one.
type DufsSource struct {
	cfg        Config
	httpClient *http.Client
}

// NewDufsSource constructs a DufsSource from the given Config.
func NewDufsSource(cfg Config) *DufsSource {
	return &DufsSource{
		cfg: Config{
			Endpoint: strings.TrimRight(cfg.Endpoint, "/"),
			Username: cfg.Username,
			Password: cfg.Password,
		},
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the source identifier.
func (s *DufsSource) Name() string {
	return "dufs"
}

// Fetch lists images served by dufs and downloads a random one.
func (s *DufsSource) Fetch(ctx context.Context) ([]byte, error) {
	names, err := s.listImageNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("dufs: listing %s: %w", s.cfg.Endpoint, err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("dufs: no images found at %s", s.cfg.Endpoint)
	}

	// #nosec G404 -- math/rand is intentional; image selection does not require cryptographic randomness
	name := names[rand.IntN(len(names))]
	slog.Info("dufs: selected image", "name", name, "endpoint", s.cfg.Endpoint, "totalImages", len(names))

	data, err := s.download(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("dufs: downloading %q from %s: %w", name, s.cfg.Endpoint, err)
	}
	return data, nil
}

// pathItem is one entry in a dufs JSON directory listing.
type pathItem struct {
	PathType string `json:"path_type"`
	Name     string `json:"name"`
}

// indexData is the subset of the dufs JSON listing response we consume.
type indexData struct {
	Paths []pathItem `json:"paths"`
}

// listImageNames returns the names of image files in the dufs served directory.
func (s *DufsSource) listImageNames(ctx context.Context) ([]string, error) {
	body, err := s.get(ctx, s.cfg.Endpoint+"/?json")
	if err != nil {
		return nil, err
	}
	var index indexData
	if err := json.Unmarshal(body, &index); err != nil {
		return nil, fmt.Errorf("parsing dufs listing: %w", err)
	}

	var names []string
	for _, item := range index.Paths {
		if isImageFile(item) {
			names = append(names, item.Name)
		}
	}
	return names, nil
}

// isImageFile reports whether a listing entry is a regular file with an image extension.
func isImageFile(item pathItem) bool {
	if item.PathType != "File" && item.PathType != "SymlinkFile" {
		return false
	}
	ext := strings.ToLower(path.Ext(item.Name))
	_, ok := imageExtensions[ext]
	return ok
}

// download fetches the raw bytes of a named file from the dufs served directory.
func (s *DufsSource) download(ctx context.Context, name string) ([]byte, error) {
	return s.get(ctx, s.cfg.Endpoint+"/"+url.PathEscape(name))
}

// get performs a GET request, adding HTTP basic auth when a username is configured.
func (s *DufsSource) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if s.cfg.Username != "" {
		req.SetBasicAuth(s.cfg.Username, s.cfg.Password)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}
