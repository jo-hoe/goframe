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

// uploadStatus mirrors the server's derived upload status (see
// internal/database.UploadStatus). Duplicated here as plain strings so the
// scheduler does not depend on the database package.
type uploadStatus string

const (
	statusProcessing uploadStatus = "processing"
	statusSucceeded  uploadStatus = "succeeded"
	statusFailed     uploadStatus = "failed"
	statusUnknown    uploadStatus = "unknown"
)

// uploadWaitTimeout bounds how long RunOnce waits for a submitted image to
// finish background processing before giving up. It must exceed the server's
// per-job processTimeout (5m) so a legitimately slow pipeline is not abandoned.
const uploadWaitTimeout = 6 * time.Minute

// uploadPollInterval is the delay between status polls while waiting for a
// submitted image to reach a terminal state.
const uploadPollInterval = time.Second

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

	uploaded, err := client.uploadImage(ctx, imageData, cfg.SourceName)
	if err != nil {
		return fmt.Errorf("uploading image: %w", err)
	}
	slog.Info("image-scheduler: uploaded new image",
		"source", cfg.SourceName, "id", uploaded.ID, "status", uploaded.Status)

	// POST returns 202 the moment the raw upload is stored; the image is
	// registered in rotation.json only when background processing finishes.
	// Wait for a terminal state so the post-upload list below includes the new
	// image — otherwise pruneOwnImages sees only the previous image, prunes
	// nothing, and two own images accumulate.
	if uploaded.Status != statusSucceeded {
		terminal, waitErr := waitForTerminal(ctx, client, uploaded.ID)
		if waitErr != nil {
			return fmt.Errorf("waiting for upload %s to finish: %w", uploaded.ID, waitErr)
		}
		if terminal == statusFailed {
			return fmt.Errorf("upload %s failed during processing", uploaded.ID)
		}
		slog.Info("image-scheduler: upload processed", "source", cfg.SourceName, "id", uploaded.ID)
	}

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

// uploadResult is the parsed response of a successful POST /api/image.
type uploadResult struct {
	ID     string
	Status uploadStatus
}

// uploadResponse mirrors the JSON body returned by POST /api/image
// (see internal/apihandler.uploadResponse).
type uploadResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	StatusURL string `json:"statusUrl"`
}

// uploadStateResponse mirrors the JSON body of GET /api/images/:id/status
// (see internal/database.UploadState).
type uploadStateResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

func (c *goframeClient) uploadImage(ctx context.Context, data []byte, sourceName string) (uploadResult, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return uploadResult{}, err
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return uploadResult{}, err
	}
	if err := writer.WriteField("source", sourceName); err != nil {
		return uploadResult{}, err
	}
	if err := writer.Close(); err != nil {
		return uploadResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/image", body)
	if err != nil {
		return uploadResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return uploadResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusAccepted &&
		resp.StatusCode != http.StatusOK {
		return uploadResult{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body2 uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&body2); err != nil {
		return uploadResult{}, fmt.Errorf("decoding upload response: %w", err)
	}
	if body2.ID == "" {
		return uploadResult{}, fmt.Errorf("upload response missing id")
	}
	return uploadResult{ID: body2.ID, Status: uploadStatus(body2.Status)}, nil
}

// getUploadStatus fetches the derived processing state for a content-addressed ID.
func (c *goframeClient) getUploadStatus(ctx context.Context, id string) (uploadStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/images/"+id+"/status", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var state uploadStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return "", fmt.Errorf("decoding status response: %w", err)
	}
	return uploadStatus(state.Status), nil
}

// waitForTerminal polls the upload status endpoint until the upload reaches a
// terminal state (succeeded or failed) or the wait budget is exhausted. It
// returns the terminal status; a timeout or a cancelled context is an error.
func waitForTerminal(ctx context.Context, client *goframeClient, id string) (uploadStatus, error) {
	waitCtx, cancel := context.WithTimeout(ctx, uploadWaitTimeout)
	defer cancel()

	ticker := time.NewTicker(uploadPollInterval)
	defer ticker.Stop()

	for {
		status, err := client.getUploadStatus(waitCtx, id)
		if err == nil {
			switch status {
			case statusSucceeded, statusFailed:
				return status, nil
			}
		}

		select {
		case <-waitCtx.Done():
			return "", fmt.Errorf("timed out after %s waiting for terminal status (last known: %q)", uploadWaitTimeout, id)
		case <-ticker.C:
		}
	}
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
