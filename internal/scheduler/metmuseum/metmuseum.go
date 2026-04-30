package metmuseum

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"

	"github.com/jo-hoe/goframe/internal/scheduler"
)

const (
	defaultSearchURL = "https://collectionapi.metmuseum.org/public/collection/v1/search"
	defaultObjectURL = "https://collectionapi.metmuseum.org/public/collection/v1/objects/%d"

	// maxFetchAttempts is the number of random objects to try before giving up.
	// Some highlighted objects have hasImages=true but an empty primaryImage field.
	maxFetchAttempts = 10
)

// MetMuseumSource fetches a random highlighted, public-domain artwork image from
// The Metropolitan Museum of Art Open Access API.
type MetMuseumSource struct {
	departmentIDs []int
	httpClient    *http.Client
	searchBaseURL string
	objectBaseURL string
}

// NewMetMuseumSource constructs a MetMuseumSource.
// departmentIDs constrains results to specific Met departments; pass nil or an
// empty slice to search across all departments.
func NewMetMuseumSource(departmentIDs []int) *MetMuseumSource {
	return &MetMuseumSource{
		departmentIDs: departmentIDs,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		searchBaseURL: defaultSearchURL,
		objectBaseURL: defaultObjectURL,
	}
}

// Name returns the source identifier.
func (m *MetMuseumSource) Name() string {
	return "metmuseum"
}

// Fetch retrieves a random highlighted, public-domain artwork image.
// It tries up to maxFetchAttempts random objects, skipping any that lack a primaryImage.
func (m *MetMuseumSource) Fetch(ctx context.Context) ([]byte, error) {
	ids, err := m.collectObjectIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching met museum object IDs: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("met museum search returned no results for departments %v", m.departmentIDs)
	}

	// #nosec G404 -- math/rand is intentional; artwork selection does not require cryptographic randomness
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })

	attempts := min(maxFetchAttempts, len(ids))
	for _, objectID := range ids[:attempts] {
		imageURL, err := m.fetchImageURL(ctx, objectID)
		if err != nil {
			continue
		}
		data, err := scheduler.FetchBytes(ctx, m.httpClient, imageURL)
		if err != nil {
			return nil, fmt.Errorf("downloading met museum object %d image: %w", objectID, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("met museum: no object with a primary image found after %d attempts", attempts)
}

// collectObjectIDs returns the union of object IDs across all configured departments.
// When no departments are configured it performs a single all-department search.
func (m *MetMuseumSource) collectObjectIDs(ctx context.Context) ([]int, error) {
	if len(m.departmentIDs) == 0 {
		return m.fetchObjectIDsForDepartment(ctx, 0)
	}

	seen := make(map[int]struct{})
	var all []int
	for _, deptID := range m.departmentIDs {
		ids, err := m.fetchObjectIDsForDepartment(ctx, deptID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				all = append(all, id)
			}
		}
	}
	return all, nil
}

// searchResult holds the fields we need from the Met search API response.
type searchResult struct {
	Total     int   `json:"total"`
	ObjectIDs []int `json:"objectIDs"`
}

// objectMeta holds the fields we need from the Met object API response.
type objectMeta struct {
	PrimaryImage string `json:"primaryImage"`
}

// fetchObjectIDsForDepartment fetches highlighted public-domain object IDs for a department.
// A departmentID of 0 searches across all departments.
func (m *MetMuseumSource) fetchObjectIDsForDepartment(ctx context.Context, departmentID int) ([]int, error) {
	u := buildSearchURL(m.searchBaseURL, departmentID)
	data, err := scheduler.FetchBytes(ctx, m.httpClient, u)
	if err != nil {
		return nil, err
	}
	return parseSearchResult(data)
}

func (m *MetMuseumSource) fetchImageURL(ctx context.Context, objectID int) (string, error) {
	u := fmt.Sprintf(m.objectBaseURL, objectID)
	data, err := scheduler.FetchBytes(ctx, m.httpClient, u)
	if err != nil {
		return "", err
	}
	return parseImageURL(data)
}

// buildSearchURL constructs the Met search URL for highlighted public-domain images.
// A departmentID of 0 omits the department filter.
func buildSearchURL(baseURL string, departmentID int) string {
	params := url.Values{}
	params.Set("hasImages", "true")
	params.Set("isPublicDomain", "true")
	params.Set("isHighlight", "true")
	params.Set("q", "*")
	if departmentID > 0 {
		params.Set("departmentId", fmt.Sprintf("%d", departmentID))
	}
	return baseURL + "?" + params.Encode()
}

func parseSearchResult(data []byte) ([]int, error) {
	var result searchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing met museum search response: %w", err)
	}
	return result.ObjectIDs, nil
}

func parseImageURL(data []byte) (string, error) {
	var meta objectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parsing met museum object response: %w", err)
	}
	if meta.PrimaryImage == "" {
		return "", fmt.Errorf("met museum object has no primary image")
	}
	return meta.PrimaryImage, nil
}
