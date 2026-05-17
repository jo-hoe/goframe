package database

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	sigv4Algorithm = "AWS4-HMAC-SHA256"
	sigv4Service   = "s3"
	sigv4Request   = "aws4_request"
)

// s3Creds holds the credentials and region for SigV4 signing.
type s3Creds struct {
	accessKey string
	secretKey string
	region    string
}

// s3Client is a minimal S3-compatible client supporting GET, PUT, DELETE and
// presigned URL generation. It uses AWS Signature Version 4.
type s3Client struct {
	endpoint   string // no trailing slash, no bucket path
	bucket     string
	creds      s3Creds
	httpClient *http.Client
	nowFn      func() time.Time
}

func newS3Client(endpoint, bucket, accessKey, secretKey, region string) *s3Client {
	return &s3Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		bucket:     bucket,
		creds:      s3Creds{accessKey: accessKey, secretKey: secretKey, region: region},
		httpClient: &http.Client{Timeout: 60 * time.Second},
		nowFn:      time.Now,
	}
}

// objectURL returns the path-style URL for a key.
func (c *s3Client) objectURL(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return c.endpoint + "/" + c.bucket + "/" + strings.Join(parts, "/")
}

// PutObject uploads data to key with the given content type.
func (c *s3Client) PutObject(ctx context.Context, key, contentType string, data []byte) error {
	rawURL := c.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("s3: building PUT request for %q: %w", key, err)
	}
	req.Header.Set("Content-Type", contentType)
	c.signRequestWithBody(req, data)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3: PUT %q: %w", key, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3: PUT %q: unexpected status %d: %s", key, resp.StatusCode, string(body))
	}
	return nil
}

// DeleteObject removes the object at key.
func (c *s3Client) DeleteObject(ctx context.Context, key string) error {
	rawURL := c.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, nil)
	if err != nil {
		return fmt.Errorf("s3: building DELETE request for %q: %w", key, err)
	}
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3: DELETE %q: %w", key, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3: DELETE %q: unexpected status %d: %s", key, resp.StatusCode, string(body))
	}
	return nil
}

// ObjectURL returns the direct (unsigned) path-style URL for a key.
// Suitable when the bucket is publicly accessible or when the caller will
// handle authentication via presigned query parameters.
func (c *s3Client) ObjectURL(key string) string {
	return c.objectURL(key)
}

// EnsureBucket creates the bucket if it does not already exist.
func (c *s3Client) EnsureBucket(ctx context.Context) error {
	bucketURL := c.endpoint + "/" + c.bucket
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, bucketURL, nil)
	if err != nil {
		return fmt.Errorf("s3: building PUT bucket request: %w", err)
	}
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3: creating bucket %q: %w", c.bucket, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// 200 = created, 409 = already exists — both are fine.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3: creating bucket %q: unexpected status %d: %s", c.bucket, resp.StatusCode, string(body))
	}
	return nil
}

// SetPublicReadPolicy sets a bucket policy allowing anonymous GET on objects
// matching the given key prefix (e.g. "images/*").
func (c *s3Client) SetPublicReadPolicy(ctx context.Context, prefix string) error {
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::%s/%s"}]}`, c.bucket, prefix)
	policyURL := c.endpoint + "/" + c.bucket + "?policy="
	body := []byte(policy)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, policyURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("s3: building PUT policy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.signRequestWithBody(req, body)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3: setting bucket policy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3: setting bucket policy: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetObject downloads the object at key and returns its body bytes.
// Returns (nil, nil) when the object does not exist (404).
func (c *s3Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	rawURL := c.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("s3: building GET request for %q: %w", key, err)
	}
	c.signRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3: GET %q: %w", key, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("s3: GET %q: unexpected status %d: %s", key, resp.StatusCode, string(body))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("s3: reading body for %q: %w", key, err)
	}
	return data, nil
}

// signRequest signs a request with an empty body using AWS SigV4.
func (c *s3Client) signRequest(req *http.Request) {
	c.signRequestWithBody(req, nil)
}

// signRequestWithBody signs a request using AWS SigV4 with the given body hash.
func (c *s3Client) signRequestWithBody(req *http.Request, body []byte) {
	if c.creds.accessKey == "" {
		return
	}
	now := c.nowFn().UTC()
	date := now.Format("20060102")
	datetime := now.Format("20060102T150405Z")

	var bodyHash string
	if body == nil {
		bodyHash = hashSHA256bytes(nil)
	} else {
		bodyHash = hashSHA256bytes(body)
	}

	req.Header.Set("x-amz-date", datetime)
	req.Header.Set("x-amz-content-sha256", bodyHash)

	var signedHeaders, canonicalHeaders string
	if ct := req.Header.Get("Content-Type"); ct != "" {
		signedHeaders = "content-type;host;x-amz-content-sha256;x-amz-date"
		canonicalHeaders = "content-type:" + ct + "\n" +
			"host:" + req.URL.Host + "\n" +
			"x-amz-content-sha256:" + bodyHash + "\n" +
			"x-amz-date:" + datetime + "\n"
	} else {
		signedHeaders = "host;x-amz-content-sha256;x-amz-date"
		canonicalHeaders = "host:" + req.URL.Host + "\n" +
			"x-amz-content-sha256:" + bodyHash + "\n" +
			"x-amz-date:" + datetime + "\n"
	}

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalRequest := req.Method + "\n" +
		canonicalURI + "\n" +
		req.URL.RawQuery + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		bodyHash

	credentialScope := date + "/" + c.creds.region + "/" + sigv4Service + "/" + sigv4Request
	stringToSign := sigv4Algorithm + "\n" +
		datetime + "\n" +
		credentialScope + "\n" +
		hashSHA256string(canonicalRequest)

	signature := hex.EncodeToString(hmacSHA256bytes(deriveSigningKey(c.creds, date), stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigv4Algorithm, c.creds.accessKey, credentialScope, signedHeaders, signature,
	))
}

func deriveSigningKey(creds s3Creds, date string) []byte {
	kDate := hmacSHA256bytes([]byte("AWS4"+creds.secretKey), date)
	kRegion := hmacSHA256bytes(kDate, creds.region)
	kService := hmacSHA256bytes(kRegion, sigv4Service)
	return hmacSHA256bytes(kService, sigv4Request)
}

func hmacSHA256bytes(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hashSHA256bytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hashSHA256string(s string) string {
	return hashSHA256bytes([]byte(s))
}
