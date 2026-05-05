package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const (
	sigv4Algorithm = "AWS4-HMAC-SHA256"
	sigv4Service   = "s3"
	sigv4Request   = "aws4_request"
)

type credentials struct {
	accessKey string
	secretKey string
	region    string
}

// signRequest adds AWS Signature Version 4 headers to req.
// Assumes an empty request body (GET only).
func signRequest(req *http.Request, creds credentials, now time.Time) {
	date := now.UTC().Format("20060102")
	datetime := now.UTC().Format("20060102T150405Z")
	bodyHash := hashSHA256("")

	req.Header.Set("x-amz-date", datetime)
	req.Header.Set("x-amz-content-sha256", bodyHash)

	// Signed headers must be in sorted order. These three are always signed.
	// "host" < "x-amz-content-sha256" < "x-amz-date" is already lexicographic order.
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-amz-content-sha256:" + bodyHash + "\n" +
		"x-amz-date:" + datetime + "\n"

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

	credentialScope := date + "/" + creds.region + "/" + sigv4Service + "/" + sigv4Request
	stringToSign := sigv4Algorithm + "\n" +
		datetime + "\n" +
		credentialScope + "\n" +
		hashSHA256(canonicalRequest)

	signature := hex.EncodeToString(hmacSHA256(deriveSigningKey(creds, date), stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigv4Algorithm, creds.accessKey, credentialScope, signedHeaders, signature,
	))
}

func deriveSigningKey(creds credentials, date string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+creds.secretKey), date)
	kRegion := hmacSHA256(kDate, creds.region)
	kService := hmacSHA256(kRegion, sigv4Service)
	return hmacSHA256(kService, sigv4Request)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hashSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
