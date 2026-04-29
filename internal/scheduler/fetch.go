package scheduler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	fetchMaxRetries = 3
	fetchBaseDelay  = 2 * time.Second
)

// FetchBytes performs a GET request and returns the response body.
// Retries up to fetchMaxRetries times with exponential backoff on 429 or 403
// (rate-limit / CloudFlare transient blocks).
func FetchBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	var lastErr error
	delay := fetchBaseDelay

	for attempt := range fetchMaxRetries + 1 {
		data, retry, err := fetchOnce(ctx, client, url)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retry || attempt == fetchMaxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}

	return nil, lastErr
}

func fetchOnce(ctx context.Context, client *http.Client, url string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 429 Too Many Requests and 403 Forbidden from CloudFlare are transient — retry.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		return nil, true, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}
