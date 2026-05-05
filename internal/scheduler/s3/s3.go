package s3

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds the parameters needed to connect to an S3-compatible bucket.
type Config struct {
	// Endpoint is the base URL of the S3-compatible service (no trailing slash, no bucket path).
	// For AWS S3: "https://s3.<region>.amazonaws.com"
	// For RustFS / MinIO: "http://rustfs:9000"
	Endpoint string
	// Bucket is the name of the bucket to fetch images from.
	Bucket string
	// Prefix is an optional key prefix to filter objects (e.g. "photos/").
	Prefix string
	// Region is the AWS region identifier (e.g. "us-east-1").
	// For RustFS, any non-empty string is accepted.
	Region string
	// AccessKey is the access key ID.
	AccessKey string
	// SecretKey is the secret access key.
	SecretKey string
}

// S3Source fetches a random image from an S3-compatible bucket (AWS S3, RustFS, MinIO, etc.).
// It lists objects under an optional prefix and downloads one at random.
type S3Source struct {
	cfg        Config
	creds      credentials
	httpClient *http.Client
	nowFn      func() time.Time // injectable for tests
}

// NewS3Source constructs an S3Source from the given Config.
func NewS3Source(cfg Config) *S3Source {
	return &S3Source{
		cfg:        Config{Endpoint: strings.TrimRight(cfg.Endpoint, "/"), Bucket: cfg.Bucket, Prefix: cfg.Prefix, Region: cfg.Region, AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey},
		creds:      credentials{accessKey: cfg.AccessKey, secretKey: cfg.SecretKey, region: cfg.Region},
		httpClient: &http.Client{Timeout: 30 * time.Second},
		nowFn:      time.Now,
	}
}

// Name returns the source identifier.
func (s *S3Source) Name() string {
	return "s3"
}

// Fetch lists objects in the bucket/prefix and downloads a random one.
func (s *S3Source) Fetch(ctx context.Context) ([]byte, error) {
	keys, err := s.listObjectKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("s3: listing objects in %s: %w", s.cfg.Bucket, err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("s3: no objects found in bucket %q with prefix %q", s.cfg.Bucket, s.cfg.Prefix)
	}

	// #nosec G404 -- math/rand is intentional; object selection does not require cryptographic randomness
	key := keys[rand.IntN(len(keys))]

	data, err := s.getObject(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("s3: downloading object %q from %s: %w", key, s.cfg.Bucket, err)
	}
	return data, nil
}

// listObjectKeys returns all object keys in the configured bucket/prefix, following pagination.
func (s *S3Source) listObjectKeys(ctx context.Context) ([]string, error) {
	var keys []string
	continuationToken := ""

	for {
		rawURL := s.listURL(continuationToken)
		resp, err := s.doSigned(ctx, rawURL)
		if err != nil {
			return nil, err
		}

		result, err := parseListResult(resp)
		if err != nil {
			return nil, err
		}
		for _, c := range result.Contents {
			keys = append(keys, c.Key)
		}
		if !result.IsTruncated {
			break
		}
		continuationToken = result.NextContinuationToken
	}
	return keys, nil
}

func (s *S3Source) listURL(continuationToken string) string {
	params := url.Values{}
	params.Set("list-type", "2")
	if s.cfg.Prefix != "" {
		params.Set("prefix", s.cfg.Prefix)
	}
	if continuationToken != "" {
		params.Set("continuation-token", continuationToken)
	}
	return s.cfg.Endpoint + "/" + s.cfg.Bucket + "?" + params.Encode()
}

func (s *S3Source) getObject(ctx context.Context, key string) ([]byte, error) {
	rawURL := s.cfg.Endpoint + "/" + s.cfg.Bucket + "/" + url.PathEscape(key)
	return s.doSigned(ctx, rawURL)
}

// doSigned performs a GET request, signing it when credentials are configured.
// For anonymous (public) buckets, credentials are empty and the request is sent unsigned.
func (s *S3Source) doSigned(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if s.creds.accessKey != "" {
		signRequest(req, s.creds, s.nowFn())
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

// listResult holds the fields needed from a ListObjectsV2 XML response.
type listResult struct {
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	IsTruncated           bool   `xml:"IsTruncated"`
}

func parseListResult(data []byte) (*listResult, error) {
	var result listResult
	if err := xml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing ListObjectsV2 response: %w", err)
	}
	return &result, nil
}
