package s3

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.RawQuery != "" && r.URL.Query().Get("list-type") == "2":
			// ListObjectsV2
			result := listResult{}
			result.Contents = []struct {
				Key string `xml:"Key"`
			}{{Key: "photos/cat.jpg"}}
			data, _ := xml.Marshal(result)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(data)
		case r.URL.Path == "/mybucket/photos%2Fcat.jpg" || r.URL.Path == "/mybucket/photos/cat.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := newTestSource(srv, "mybucket", "")
	data, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func TestFetch_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	source := newTestSource(srv, "mybucket", "")
	_, err := source.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetch_GetObjectError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			result := listResult{}
			result.Contents = []struct {
				Key string `xml:"Key"`
			}{{Key: "photos/cat.jpg"}}
			data, _ := xml.Marshal(result)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(data)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	source := newTestSource(srv, "mybucket", "")
	_, err := source.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetch_EmptyBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := listResult{}
		data, _ := xml.Marshal(result)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	source := newTestSource(srv, "mybucket", "")
	_, err := source.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for empty bucket, got nil")
	}
}

func TestFetch_AnonymousAccess(t *testing.T) {
	imageBytes := []byte("public-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "expected no auth", http.StatusBadRequest)
			return
		}
		switch {
		case r.URL.Query().Get("list-type") == "2":
			result := listResult{}
			result.Contents = []struct {
				Key string `xml:"Key"`
			}{{Key: "public.jpg"}}
			data, _ := xml.Marshal(result)
			_, _ = w.Write(data)
		default:
			_, _ = w.Write(imageBytes)
		}
	}))
	defer srv.Close()

	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := NewS3Source(Config{
		Endpoint:  srv.URL,
		Bucket:    "public-bucket",
		Region:    "us-east-1",
	})
	s.httpClient = srv.Client()
	s.nowFn = func() time.Time { return fixedTime }

	data, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

func newTestSource(srv *httptest.Server, bucket, prefix string) *S3Source {
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := NewS3Source(Config{
		Endpoint:  srv.URL,
		Bucket:    bucket,
		Prefix:    prefix,
		Region:    "us-east-1",
		AccessKey: "AKIATEST",
		SecretKey: "testsecret",
	})
	s.httpClient = srv.Client()
	s.nowFn = func() time.Time { return fixedTime }
	return s
}

// TestFetch_PublicBucket_EndToEnd hits the real inaturalist-open-data public S3 bucket.
// It is skipped in short mode (-short) to avoid network access in CI.
func TestFetch_PublicBucket_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}

	source := NewS3Source(Config{
		Endpoint: "https://s3.us-east-1.amazonaws.com",
		Bucket:   "inaturalist-open-data",
		Prefix:   "photos/100/",
		Region:   "us-east-1",
	})

	data, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty image data")
	}
}
