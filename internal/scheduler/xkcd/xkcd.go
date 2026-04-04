package xkcd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

const (
	latestComicURL = "https://xkcd.com/info.0.json"
	comicURLFormat = "https://xkcd.com/%d/info.0.json"

	// comicNum404 is the xkcd comic that is intentionally missing (a meta-joke about HTTP 404).
	comicNum404 = 404
)

// XKCDSource fetches a random XKCD comic image.
type XKCDSource struct {
	httpClient *http.Client
}

// NewXKCDSource constructs an XKCDSource with a sensible default HTTP client.
func NewXKCDSource() *XKCDSource {
	return &XKCDSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the source identifier.
func (x *XKCDSource) Name() string {
	return "xkcd"
}

// Fetch retrieves a random XKCD comic image as raw PNG/JPEG bytes.
func (x *XKCDSource) Fetch(ctx context.Context) ([]byte, error) {
	latest, err := x.fetchComicMeta(ctx, latestComicURL)
	if err != nil {
		return nil, fmt.Errorf("fetching latest xkcd comic metadata: %w", err)
	}

	comicNum := randomComicNumber(latest.Num)

	url := fmt.Sprintf(comicURLFormat, comicNum)
	comic, err := x.fetchComicMeta(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching xkcd comic %d metadata: %w", comicNum, err)
	}

	data, err := x.fetchImageBytes(ctx, comic.ImgURL)
	if err != nil {
		return nil, fmt.Errorf("downloading xkcd comic %d image: %w", comicNum, err)
	}
	return data, nil
}

// comicMeta holds the fields we need from the XKCD JSON API.
type comicMeta struct {
	Num    int    `json:"num"`
	ImgURL string `json:"img"`
}

func (x *XKCDSource) fetchComicMeta(ctx context.Context, url string) (*comicMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := x.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	var meta comicMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (x *XKCDSource) fetchImageBytes(ctx context.Context, imgURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := x.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching image from %s", resp.StatusCode, imgURL)
	}

	return io.ReadAll(resp.Body)
}

// randomComicNumber returns a random comic number in [1, latestNum] excluding the missing comic 404.
func randomComicNumber(latestNum int) int {
	if latestNum <= 1 {
		return 1
	}
	// Exclude comic 404 by mapping the range [1, latestNum-1] past the gap.
	// #nosec G404 -- math/rand is intentional; comic selection does not require cryptographic randomness
	n := rand.IntN(latestNum-1) + 1 // [1, latestNum-1]
	if n >= comicNum404 {
		n++
	}
	if n > latestNum {
		n = latestNum
	}
	return n
}
