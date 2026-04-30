package metmuseum

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSearchResult_Valid(t *testing.T) {
	data, _ := json.Marshal(searchResult{Total: 2, ObjectIDs: []int{1, 2}})
	ids, err := parseSearchResult(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("unexpected ids: %v", ids)
	}
}

func TestParseSearchResult_Invalid(t *testing.T) {
	_, err := parseSearchResult([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseImageURL_Valid(t *testing.T) {
	data, _ := json.Marshal(objectMeta{PrimaryImage: "https://example.com/img.jpg"})
	u, err := parseImageURL(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != "https://example.com/img.jpg" {
		t.Errorf("unexpected URL: %q", u)
	}
}

func TestParseImageURL_Missing(t *testing.T) {
	data, _ := json.Marshal(objectMeta{PrimaryImage: ""})
	_, err := parseImageURL(data)
	if err == nil {
		t.Fatal("expected error for missing primary image, got nil")
	}
}

func TestParseImageURL_Invalid(t *testing.T) {
	_, err := parseImageURL([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestBuildSearchURL_NoDepartment(t *testing.T) {
	u := buildSearchURL(defaultSearchURL, 0)
	for _, param := range []string{"hasImages=true", "isPublicDomain=true", "isHighlight=true"} {
		if !strings.Contains(u, param) {
			t.Errorf("expected URL to contain %q, got %q", param, u)
		}
	}
	if strings.Contains(u, "departmentId") {
		t.Errorf("expected no departmentId when ID is 0, got %q", u)
	}
}

func TestBuildSearchURL_WithDepartment(t *testing.T) {
	u := buildSearchURL(defaultSearchURL, 11)
	if !strings.Contains(u, "departmentId=11") {
		t.Errorf("expected departmentId=11 in URL, got %q", u)
	}
}

func TestFetchObjectIDsForDepartment_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	source := newTestSource(srv, nil)
	_, err := source.fetchObjectIDsForDepartment(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestCollectObjectIDs_Deduplication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both department searches return the same IDs — deduplication must collapse them.
		data, _ := json.Marshal(searchResult{Total: 2, ObjectIDs: []int{1, 2}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	source := newTestSource(srv, []int{11, 6})
	ids, err := source.collectObjectIDs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 deduplicated IDs, got %d: %v", len(ids), ids)
	}
}

func TestFetch_Success(t *testing.T) {
	imageBytes := []byte("fake-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			data, _ := json.Marshal(searchResult{Total: 1, ObjectIDs: []int{42}})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		case "/objects/42":
			data, _ := json.Marshal(objectMeta{PrimaryImage: "http://" + r.Host + "/img/42.jpg"})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		case "/img/42.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := newTestSource(srv, nil)
	data, err := source.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != string(imageBytes) {
		t.Errorf("expected %q, got %q", imageBytes, data)
	}
}

// newTestSource constructs a MetMuseumSource pointed at the given test server.
func newTestSource(srv *httptest.Server, departmentIDs []int) *MetMuseumSource {
	return &MetMuseumSource{
		departmentIDs: departmentIDs,
		httpClient:    srv.Client(),
		searchBaseURL: srv.URL + "/search",
		objectBaseURL: srv.URL + "/objects/%d",
	}
}
