package s3

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"
)

var testCreds = credentials{accessKey: "AKIATEST", secretKey: "testsecret", region: "eu-central-1"}
var testTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// TestSignRequest_PlusInKey is the regression test for the double-encoding bug:
// keys containing '+' were encoded as %2B in the request URL, but EscapedPath()
// would then re-encode that to %252B in the canonical URI, causing a signature
// mismatch (403) against AWS which expects single-encoded %2B.
func TestSignRequest_PlusInKey(t *testing.T) {
	rawURL := "https://s3.eu-central-1.amazonaws.com/goframe/dinosaurs-attack%2F31%2Bour%2Bforces%2B-%2Bflattened.jpg"
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	signRequest(req, testCreds, testTime)

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("Authorization header not set")
	}
	expectedSig := signatureForURI("/goframe/dinosaurs-attack%2F31%2Bour%2Bforces%2B-%2Bflattened.jpg", testCreds, testTime)
	if !strings.Contains(auth, "Signature="+expectedSig) {
		t.Errorf("Authorization header has wrong signature.\ngot:  %s\nwant Signature=%s", auth, expectedSig)
	}
}

// TestSignRequest_SlashInKey verifies that a subfolder path with encoded slash is not altered.
func TestSignRequest_SlashInKey(t *testing.T) {
	rawURL := "https://s3.eu-central-1.amazonaws.com/goframe/subfolder%2Fimage.jpg"
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	signRequest(req, testCreds, testTime)

	auth := req.Header.Get("Authorization")
	expectedSig := signatureForURI("/goframe/subfolder%2Fimage.jpg", testCreds, testTime)
	if !strings.Contains(auth, "Signature="+expectedSig) {
		t.Errorf("Authorization header has wrong signature.\ngot:  %s\nwant Signature=%s", auth, expectedSig)
	}
}

// TestSignRequest_PlainPath verifies that an unencoded path falls back to EscapedPath() correctly.
func TestSignRequest_PlainPath(t *testing.T) {
	rawURL := "https://s3.eu-central-1.amazonaws.com/goframe/plain.jpg"
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	signRequest(req, testCreds, testTime)

	auth := req.Header.Get("Authorization")
	expectedSig := signatureForURI("/goframe/plain.jpg", testCreds, testTime)
	if !strings.Contains(auth, "Signature="+expectedSig) {
		t.Errorf("Authorization header has wrong signature.\ngot:  %s\nwant Signature=%s", auth, expectedSig)
	}
}

// signatureForURI computes the expected HMAC signature for a given canonical URI.
func signatureForURI(canonicalURI string, creds credentials, now time.Time) string {
	date := now.UTC().Format("20060102")
	datetime := now.UTC().Format("20060102T150405Z")
	bodyHash := hashSHA256("")

	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:s3.eu-central-1.amazonaws.com\n" +
		"x-amz-content-sha256:" + bodyHash + "\n" +
		"x-amz-date:" + datetime + "\n"

	canonicalRequest := "GET\n" +
		canonicalURI + "\n" +
		"\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		bodyHash

	credentialScope := date + "/" + creds.region + "/" + sigv4Service + "/" + sigv4Request
	stringToSign := sigv4Algorithm + "\n" +
		datetime + "\n" +
		credentialScope + "\n" +
		hashSHA256(canonicalRequest)

	return hex.EncodeToString(hmacSHA256(deriveSigningKey(creds, date), stringToSign))
}
