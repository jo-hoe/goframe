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

// OnExternalImages controls scheduler behaviour when external images are present
// (images not owned by this scheduler or any member of its group).
type OnExternalImages string

const (
	// OnExternalImagesIgnore uploads normally, leaving external images untouched (default).
	OnExternalImagesIgnore OnExternalImages = "ignore"
	// OnExternalImagesTakeover deletes all external images after a successful upload.
	OnExternalImagesTakeover OnExternalImages = "takeover"
	// OnExternalImagesYield deletes own images and skips the upload when external images exist.
	OnExternalImagesYield OnExternalImages = "yield"
)

// Config holds all parameters required for a single image scheduler run.
type Config struct {
	// GoframeBaseURL is the base URL of the goframe service.
	GoframeBaseURL string
	// SourceName is the unique identity of this image scheduler instance.
	SourceName string
	// Group is an optional group name. When non-empty, a successful upload causes all
	// images owned by other members in the same group to be deleted.
	Group string
	// GroupMembers lists the source names of all schedulers sharing Group, including
	// this scheduler's own SourceName. Populated by the operator.
	GroupMembers []string
	// OnExternalImages controls what happens when external images are present.
	OnExternalImages OnExternalImages
	// Source is the image source used to fetch a new image.
	Source ImageSource
	// Commands is an optional pipeline applied after PNG conversion.
	Commands []imageprocessing.CommandConfig
}

// RunOnce executes one image scheduler cycle:
//  1. List images; check external image policy.
//  2. Fetch, convert, process, and upload a new image.
//  3. Evict group peers and external images as configured.
//  4. Delete own old images (always keep exactly 1).
func RunOnce(ctx context.Context, cfg Config) error {
	client := newGoframeClient(cfg.GoframeBaseURL)

	images, err := client.listImages(ctx)
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	if cfg.OnExternalImages == OnExternalImagesYield {
		if hasExternalImages(images, cfg.SourceName, cfg.GroupMembers) {
			slog.Info("image-scheduler: external images present, yielding",
				"source", cfg.SourceName)
			return deleteOwnImages(ctx, client, images, cfg.SourceName)
		}
	}

	imageData, err := cfg.Source.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("fetching image from source %q: %w", cfg.Source.Name(), err)
	}
	slog.Info("image-scheduler: fetched image", "source", cfg.SourceName, "bytes", len(imageData))

	pngCmd, err := imageprocessing.DefaultRegistry.Create("PngConverterCommand", nil)
	if err != nil {
		return fmt.Errorf("creating PNG converter: %w", err)
	}
	imageData, err = pngCmd.Execute(imageData)
	if err != nil {
		return fmt.Errorf("converting image to PNG from source %q: %w", cfg.Source.Name(), err)
	}
	slog.Info("image-scheduler: converted to PNG", "source", cfg.SourceName, "bytes", len(imageData))

	if len(cfg.Commands) > 0 {
		imageData, err = imageprocessing.ExecuteCommands(imageData, cfg.Commands)
		if err != nil {
			return fmt.Errorf("processing image from source %q: %w", cfg.Source.Name(), err)
		}
		slog.Info("image-scheduler: applied command pipeline", "source", cfg.SourceName, "commands", len(cfg.Commands), "bytes", len(imageData))
	}

	if err := client.uploadImage(ctx, imageData, cfg.SourceName); err != nil {
		return fmt.Errorf("uploading image: %w", err)
	}
	slog.Info("image-scheduler: uploaded new image", "source", cfg.SourceName)

	images, err = client.listImages(ctx)
	if err != nil {
		return fmt.Errorf("listing images after upload: %w", err)
	}

	if cfg.OnExternalImages == OnExternalImagesTakeover {
		if err := deleteExternalImages(ctx, client, images, cfg.SourceName, cfg.GroupMembers); err != nil {
			return err
		}
	}

	if cfg.Group != "" {
		if err := evictGroupPeers(ctx, client, images, cfg.SourceName, cfg.GroupMembers); err != nil {
			return err
		}
	}

	return pruneOwnImages(ctx, client, images, cfg.SourceName)
}

// hasExternalImages returns true if any image is not owned by sourceName or a group member.
func hasExternalImages(images []apiImageItem, sourceName string, groupMembers []string) bool {
	known := makeKnownSet(sourceName, groupMembers)
	for _, img := range images {
		if _, ok := known[img.Source]; !ok {
			return true
		}
	}
	return false
}

// deleteExternalImages deletes all images not owned by sourceName or a group member.
func deleteExternalImages(ctx context.Context, client *goframeClient, images []apiImageItem, sourceName string, groupMembers []string) error {
	known := makeKnownSet(sourceName, groupMembers)
	var errs []string
	for _, img := range images {
		if _, ok := known[img.Source]; ok {
			continue
		}
		if err := client.deleteImage(ctx, img.ID); err != nil {
			errs = append(errs, fmt.Sprintf("delete %s (source %q): %v", img.ID, img.Source, err))
			continue
		}
		slog.Info("image-scheduler: deleted external image", "id", img.ID, "source", img.Source, "deletedBy", sourceName)
	}
	if len(errs) > 0 {
		return fmt.Errorf("deleting external images: %s", strings.Join(errs, "; "))
	}
	return nil
}

// deleteOwnImages deletes all images owned by sourceName.
func deleteOwnImages(ctx context.Context, client *goframeClient, images []apiImageItem, sourceName string) error {
	var errs []string
	for _, img := range filterBySource(images, sourceName) {
		if err := client.deleteImage(ctx, img.ID); err != nil {
			errs = append(errs, fmt.Sprintf("delete %s: %v", img.ID, err))
			continue
		}
		slog.Info("image-scheduler: deleted own image (yield)", "id", img.ID, "source", sourceName)
	}
	if len(errs) > 0 {
		return fmt.Errorf("deleting own images: %s", strings.Join(errs, "; "))
	}
	return nil
}

// evictGroupPeers deletes all images owned by other members of the group.
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

// pruneOwnImages keeps only the newest image owned by sourceName (always exactly 1).
func pruneOwnImages(ctx context.Context, client *goframeClient, images []apiImageItem, sourceName string) error {
	ownImages := filterBySource(images, sourceName)
	if len(ownImages) <= 1 {
		return nil
	}

	// Images are returned by the API in order (oldest first); keep the last one.
	var errs []string
	for _, img := range ownImages[:len(ownImages)-1] {
		if err := client.deleteImage(ctx, img.ID); err != nil {
			errs = append(errs, fmt.Sprintf("delete %s: %v", img.ID, err))
			continue
		}
		slog.Info("image-scheduler: deleted old image", "id", img.ID, "source", sourceName)
	}
	if len(errs) > 0 {
		return fmt.Errorf("pruning images for source %q: %s", sourceName, strings.Join(errs, "; "))
	}
	return nil
}

// makeKnownSet builds a set of source names that are considered "internal" (self + group).
func makeKnownSet(sourceName string, groupMembers []string) map[string]struct{} {
	known := make(map[string]struct{}, len(groupMembers)+1)
	known[sourceName] = struct{}{}
	for _, m := range groupMembers {
		known[m] = struct{}{}
	}
	return known
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
