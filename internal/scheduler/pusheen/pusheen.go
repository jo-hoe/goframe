package pusheen

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"regexp"
	"time"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const (
	// comicsAPIURL fetches a single random comic post from the WordPress REST API.
	// Category 4 = Comics, 620 total comics.
	comicsAPIURL = "https://pusheen.com/wp-json/wp/v2/posts?categories=4&per_page=1&page=%d&_fields=content"
	comicCount   = 620
)

// imgSrcPattern extracts the GIF URL from the WordPress REST API content field.
// The rendered HTML inside the JSON is backslash-escaped, so quotes appear as \".
var imgSrcPattern = regexp.MustCompile(`src=\\"(https://pusheen\.com/wp-content/uploads/[^\\"]+\.gif)\\"`)

// escapedSlashRe matches JSON-escaped forward slashes (\/) for normalisation.
var escapedSlashRe = regexp.MustCompile(`\\/`)

// PusheenSource fetches a random Pusheen comic GIF via the WordPress REST API.
type PusheenSource struct {
	httpClient *http.Client
}

// NewPusheenSource constructs a PusheenSource with a sensible default HTTP client.
func NewPusheenSource() *PusheenSource {
	return &PusheenSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the source identifier.
func (p *PusheenSource) Name() string {
	return "pusheen"
}

// Fetch retrieves a random Pusheen comic GIF as raw bytes.
func (p *PusheenSource) Fetch(ctx context.Context) ([]byte, error) {
	// #nosec G404 -- math/rand is intentional; comic selection does not require cryptographic randomness
	page := rand.IntN(comicCount) + 1
	apiURL := fmt.Sprintf(comicsAPIURL, page)

	body, err := scheduler.FetchBytes(ctx, p.httpClient, apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching pusheen comic list (page %d): %w", page, err)
	}

	imgURL, err := extractImageURL(body)
	if err != nil {
		return nil, fmt.Errorf("extracting image URL from pusheen page %d: %w", page, err)
	}

	data, err := scheduler.FetchBytes(ctx, p.httpClient, imgURL)
	if err != nil {
		return nil, fmt.Errorf("downloading pusheen image %s: %w", imgURL, err)
	}
	return data, nil
}

func extractImageURL(body []byte) (string, error) {
	normalised := escapedSlashRe.ReplaceAll(body, []byte("/"))
	sub := imgSrcPattern.FindSubmatch(normalised)
	if sub == nil {
		return "", fmt.Errorf("no pusheen GIF src found in response")
	}
	return string(sub[1]), nil
}
