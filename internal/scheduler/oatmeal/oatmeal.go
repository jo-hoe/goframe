package oatmeal

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
	indexURL    = "https://theoatmeal.com/comics"
	pageURL     = "https://theoatmeal.com/c2index/page:%d"
	pageCount   = 9
	maxAttempts = 10
)

// slugPattern matches comic hrefs like /comics/some_slug on the index pages.
var slugPattern = regexp.MustCompile(`href="/comics/([^"]+)"`)

// imgPattern matches the first comic image URL from a comic page.
// Only accepts images under /comics/{slug}/ on the theoatmeal S3 bucket.
var imgPattern = regexp.MustCompile(`src="(https://s3\.amazonaws\.com/theoatmeal-img/comics/[^/]+/[^"]+\.(?:png|jpg|gif))"`)

// OatmealSource fetches a random single-panel The Oatmeal comic.
type OatmealSource struct {
	httpClient *http.Client
}

// NewOatmealSource constructs an OatmealSource with a sensible default HTTP client.
func NewOatmealSource() *OatmealSource {
	return &OatmealSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the source identifier.
func (o *OatmealSource) Name() string {
	return "oatmeal"
}

// Fetch picks a random single-panel Oatmeal comic and returns its image bytes.
// Comics with multiple image panels are skipped.
func (o *OatmealSource) Fetch(ctx context.Context) ([]byte, error) {
	slugs, err := o.fetchSlugs(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching oatmeal comic list: %w", err)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("no oatmeal comics found")
	}

	// Shuffle so we don't always retry the same candidates.
	// #nosec G404 -- math/rand is intentional; comic selection does not require cryptographic randomness
	rand.Shuffle(len(slugs), func(i, j int) { slugs[i], slugs[j] = slugs[j], slugs[i] })

	attempts := maxAttempts
	if len(slugs) < attempts {
		attempts = len(slugs)
	}
	for _, slug := range slugs[:attempts] {
		imgURL, err := o.fetchSinglePanelImageURL(ctx, indexURL+"/"+slug)
		if err != nil {
			// Multi-panel or unparseable — try next.
			continue
		}
		data, err := scheduler.FetchBytes(ctx, o.httpClient, imgURL)
		if err != nil {
			return nil, fmt.Errorf("downloading oatmeal image %s: %w", imgURL, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("no single-panel oatmeal comic found after %d attempts", attempts)
}

// fetchSlugs scrapes all comic slugs from all index pages.
func (o *OatmealSource) fetchSlugs(ctx context.Context) ([]string, error) {
	urls := make([]string, 0, pageCount)
	urls = append(urls, indexURL)
	for p := 2; p <= pageCount; p++ {
		urls = append(urls, fmt.Sprintf(pageURL, p))
	}
	return o.fetchSlugsFromURLs(ctx, urls)
}

// fetchSlugsFromURLs scrapes comic slugs from the given list of index page URLs.
func (o *OatmealSource) fetchSlugsFromURLs(ctx context.Context, urls []string) ([]string, error) {
	seen := make(map[string]struct{})
	var slugs []string
	for _, u := range urls {
		body, err := scheduler.FetchBytes(ctx, o.httpClient, u)
		if err != nil {
			return nil, fmt.Errorf("fetching index page %s: %w", u, err)
		}
		for _, m := range slugPattern.FindAllSubmatch(body, -1) {
			slug := string(m[1])
			if _, ok := seen[slug]; !ok {
				seen[slug] = struct{}{}
				slugs = append(slugs, slug)
			}
		}
	}
	return slugs, nil
}

// fetchSinglePanelImageURL fetches a comic page and returns the image URL,
// returning an error if the comic contains multiple panels.
func (o *OatmealSource) fetchSinglePanelImageURL(ctx context.Context, comicURL string) (string, error) {
	body, err := scheduler.FetchBytes(ctx, o.httpClient, comicURL)
	if err != nil {
		return "", err
	}
	matches := imgPattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no comic image found at %q", comicURL)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("skipping multi-panel comic at %q (%d panels)", comicURL, len(matches))
	}
	return string(matches[0][1]), nil
}
