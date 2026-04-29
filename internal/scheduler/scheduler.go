package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/jo-hoe/goframe/internal/imageprocessing"

	// Import imageprocessing to trigger init() registrations for all commands.
	_ "github.com/jo-hoe/goframe/internal/imageprocessing"
)

// Config holds all parameters required for a single image scheduler run.
type Config struct {
	// GoframeBaseURL is the base URL of the goframe service (e.g. "http://goframe.default.svc.cluster.local").
	GoframeBaseURL string
	// SourceName is the unique identity of this image scheduler instance.
	SourceName string
	// KeepCount is the maximum number of images owned by this image scheduler to retain (must be >= 1).
	KeepCount int
	// ExclusionGroup is an optional group name. When non-empty, a successful upload causes all
	// images owned by other members listed in GroupMembers to be deleted.
	ExclusionGroup string
	// GroupMembers lists the source names of all schedulers sharing ExclusionGroup, including
	// this scheduler's own SourceName. Ignored when ExclusionGroup is empty.
	GroupMembers []string
	// Source is the image source used to fetch a new image.
	Source ImageSource
	// Commands is an optional pipeline applied after PNG conversion.
	Commands []imageprocessing.CommandConfig
}

// RunOnce executes one image scheduler cycle:
//  1. Fetch a new image from the configured source.
//  2. Convert to PNG (always).
//  3. Apply the configured command pipeline (if any).
//  4. Upload the processed image to goframe.
//  5. On success: if ExclusionGroup is set, delete all images owned by other group members.
//  6. On success: delete own images exceeding KeepCount (oldest first).
func RunOnce(ctx context.Context, cfg Config) error {
	client := newGoframeClient(cfg.GoframeBaseURL)

	imageData, err := cfg.Source.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("fetching image from source %q: %w", cfg.Source.Name(), err)
	}

	pngCmd, err := imageprocessing.DefaultRegistry.Create("PngConverterCommand", nil)
	if err != nil {
		return fmt.Errorf("creating PNG converter: %w", err)
	}
	imageData, err = pngCmd.Execute(imageData)
	if err != nil {
		return fmt.Errorf("converting image to PNG from source %q: %w", cfg.Source.Name(), err)
	}

	if len(cfg.Commands) > 0 {
		imageData, err = imageprocessing.ExecuteCommands(imageData, cfg.Commands)
		if err != nil {
			return fmt.Errorf("processing image from source %q: %w", cfg.Source.Name(), err)
		}
	}

	if err := client.uploadImage(ctx, imageData, cfg.SourceName); err != nil {
		return fmt.Errorf("uploading image: %w", err)
	}
	slog.Info("image-scheduler: uploaded new image", "source", cfg.SourceName)

	images, err := client.listImages(ctx)
	if err != nil {
		return fmt.Errorf("listing images after upload: %w", err)
	}

	if cfg.ExclusionGroup != "" {
		if err := evictGroupPeers(ctx, client, images, cfg.SourceName, cfg.GroupMembers); err != nil {
			return err
		}
	}

	return pruneOwnImages(ctx, client, images, cfg.SourceName, cfg.KeepCount)
}

// evictGroupPeers deletes all images owned by other members of the exclusion group.
func evictGroupPeers(ctx context.Context, client *goframeClient, images []apiImageItem, ownSource string, groupMembers []string) error {
	var errs []string
	for _, member := range groupMembers {
		if member == ownSource {
			continue
		}
		for _, img := range filterBySource(images, member) {
			if err := client.deleteImage(ctx, img.ID); err != nil {
				errs = append(errs, fmt.Sprintf("delete %s (source %q): %v", img.ID, member, err))
				continue
			}
			slog.Info("image-scheduler: evicted peer image", "id", img.ID, "source", member, "evictedBy", ownSource)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("evicting group peers: %s", strings.Join(errs, "; "))
	}
	return nil
}

// pruneOwnImages deletes the oldest images owned by this image scheduler when the count exceeds keepCount.
func pruneOwnImages(ctx context.Context, client *goframeClient, images []apiImageItem, sourceName string, keepCount int) error {
	ownImages := filterBySource(images, sourceName)
	excess := len(ownImages) - keepCount
	if excess <= 0 {
		return nil
	}

	var errs []string
	for _, img := range ownImages[:excess] {
		if err := client.deleteImage(ctx, img.ID); err != nil {
			errs = append(errs, fmt.Sprintf("delete %s: %v", img.ID, err))
			continue
		}
		slog.Info("image-scheduler: deleted excess image", "id", img.ID, "source", sourceName)
	}

	if len(errs) > 0 {
		return fmt.Errorf("pruning images for source %q: %s", sourceName, strings.Join(errs, "; "))
	}
	return nil
}

// filterBySource returns only the images matching the given source label.
func filterBySource(images []apiImageItem, source string) []apiImageItem {
	result := make([]apiImageItem, 0, len(images))
	for _, img := range images {
		if img.Source == source {
			result = append(result, img)
		}
	}
	return result
}

// apiImageItem mirrors the JSON shape returned by GET /api/images.
type apiImageItem struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Source    string    `json:"source"`
}

// goframeClient is a typed HTTP client for the goframe REST API.
type goframeClient struct {
	baseURL    string
	httpClient *http.Client
}

func newGoframeClient(baseURL string) *goframeClient {
	return &goframeClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *goframeClient) listImages(ctx context.Context) ([]apiImageItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/images", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var items []apiImageItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *goframeClient) uploadImage(ctx context.Context, data []byte, sourceName string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return err
	}
	if err := writer.WriteField("source", sourceName); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/image", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (c *goframeClient) deleteImage(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/images/"+id, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
