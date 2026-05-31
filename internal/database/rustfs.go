package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

const rotationStateKey = "rotation.json"

// imageMetadata holds the per-image data stored inside rotation.json.
type imageMetadata struct {
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

// rotationState is the JSON structure stored as rotation.json in RustFS.
// It is the single source of truth shared between the server and the operator.
// The current image is always OrderedIDs[0].
type rotationState struct {
	LastRotated time.Time                `json:"last_rotated"`
	OrderedIDs  []string                 `json:"ordered_ids"`
	Images      map[string]imageMetadata `json:"images"`
}

// RustFSDatabase implements DatabaseService using RustFS (S3-compatible) for
// image blobs and rotation state. It embeds RotationStateClient for shared
// rotation.json read/write logic.
type RustFSDatabase struct {
	*RotationStateClient
	imageBaseURL string
}

// NewRustFSDatabase connects to the RustFS endpoint and ensures the bucket exists.
// bucket is the S3 bucket name used for image objects.
// imageBaseURL is the browser-facing URL prefix for image assets (e.g. "/images").
func NewRustFSDatabase(endpoint, bucket, accessKey, secretKey, region, imageBaseURL string) (DatabaseService, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("rustfs: endpoint must not be empty")
	}
	if bucket == "" {
		return nil, fmt.Errorf("rustfs: bucket must not be empty")
	}
	if imageBaseURL == "" {
		imageBaseURL = "/images"
	}
	if region == "" {
		region = "us-east-1"
	}

	s3 := newS3Client(endpoint, bucket, accessKey, secretKey, region)

	if err := s3.EnsureBucket(context.Background()); err != nil {
		return nil, fmt.Errorf("rustfs: ensuring bucket %q exists: %w", bucket, err)
	}

	if err := s3.SetPublicReadPolicy(context.Background(), "images/*"); err != nil {
		slog.Warn("rustfs: could not set public read policy (anonymous image access may not work via ingress)", "error", err)
	}

	return &RustFSDatabase{
		RotationStateClient: &RotationStateClient{s3: s3},
		imageBaseURL:        imageBaseURL,
	}, nil
}

// Close is a no-op; RustFSDatabase holds no local resources.
func (r *RustFSDatabase) Close() error { return nil }

// imageOriginalKey returns the S3 object key for the original image blob.
func imageOriginalKey(id string) string { return "images/" + id + "/original.png" }

// imageProcessedKey returns the S3 object key for the processed image blob.
func imageProcessedKey(id string) string { return "images/" + id + "/processed.png" }

// CreateImage uploads blobs to RustFS, then atomically registers the image in
// rotation.json. When afterID is empty the image is appended; otherwise it is
// inserted immediately after that image in the ordered list.
func (r *RustFSDatabase) CreateImage(ctx context.Context, original, processed []byte, createdAt time.Time, source, afterID string) (string, error) {
	if original == nil {
		return "", fmt.Errorf("original image data cannot be nil")
	}
	if processed == nil {
		return "", fmt.Errorf("processed image data cannot be nil")
	}

	id, err := generateID()
	if err != nil {
		return "", err
	}

	if err := r.s3.PutObject(ctx, imageOriginalKey(id), "image/png", original); err != nil {
		return "", fmt.Errorf("rustfs: uploading original for %s: %w", id, err)
	}
	if err := r.s3.PutObject(ctx, imageProcessedKey(id), "image/png", processed); err != nil {
		_ = r.s3.DeleteObject(ctx, imageOriginalKey(id))
		return "", fmt.Errorf("rustfs: uploading processed for %s: %w", id, err)
	}

	rs, err := r.getRotationState(ctx)
	if err != nil {
		return "", fmt.Errorf("rustfs: reading rotation state for create: %w", err)
	}
	if rs.Images == nil {
		rs.Images = make(map[string]imageMetadata)
	}
	rs.Images[id] = imageMetadata{CreatedAt: createdAt.UTC(), Source: source}
	rs.OrderedIDs = insertIDAfter(rs.OrderedIDs, id, afterID)
	if err := r.putRotationState(ctx, rs); err != nil {
		return "", fmt.Errorf("rustfs: updating rotation state after create: %w", err)
	}

	return id, nil
}

// GetImageMetadata returns all image metadata in current display order (index 0 = today).
func (r *RustFSDatabase) GetImageMetadata(ctx context.Context) ([]*Image, error) {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return nil, fmt.Errorf("rustfs: reading rotation state for metadata: %w", err)
	}
	images := make([]*Image, 0, len(rs.OrderedIDs))
	for _, id := range rs.OrderedIDs {
		meta := rs.Images[id]
		images = append(images, &Image{
			ID:        id,
			CreatedAt: meta.CreatedAt,
			Source:    meta.Source,
		})
	}
	return images, nil
}

// GetImageByID returns metadata for a single image.
func (r *RustFSDatabase) GetImageByID(ctx context.Context, id string) (*Image, error) {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return nil, fmt.Errorf("rustfs: reading rotation state for GetImageByID: %w", err)
	}
	meta, ok := rs.Images[id]
	if !ok {
		return nil, fmt.Errorf("image not found: %s", id)
	}
	return &Image{ID: id, CreatedAt: meta.CreatedAt, Source: meta.Source}, nil
}

// DeleteImage removes the image from rotation.json and deletes its blobs from RustFS.
func (r *RustFSDatabase) DeleteImage(ctx context.Context, id string) error {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return fmt.Errorf("rustfs: reading rotation state for delete: %w", err)
	}
	if _, ok := rs.Images[id]; !ok {
		return fmt.Errorf("image not found: %s", id)
	}
	delete(rs.Images, id)
	rs.OrderedIDs = removeID(rs.OrderedIDs, id)
	if err := r.putRotationState(ctx, rs); err != nil {
		return fmt.Errorf("rustfs: updating rotation state after delete: %w", err)
	}

	_ = r.s3.DeleteObject(ctx, imageOriginalKey(id))
	_ = r.s3.DeleteObject(ctx, imageProcessedKey(id))
	return nil
}

// UpdateOrder replaces the display order with the given ID slice and writes
// the result to rotation.json.
func (r *RustFSDatabase) UpdateOrder(ctx context.Context, order []string) error {
	if len(order) == 0 {
		return nil
	}
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return fmt.Errorf("rustfs: reading rotation state for UpdateOrder: %w", err)
	}
	rs.OrderedIDs = order
	return r.putRotationState(ctx, rs)
}

// GetRotationOrderedIDs returns the full ordered ID list from rotation.json.
func (r *RustFSDatabase) GetRotationOrderedIDs(ctx context.Context) ([]string, error) {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return nil, fmt.Errorf("rustfs: reading rotation state for ordered IDs: %w", err)
	}
	return rs.OrderedIDs, nil
}

// GetCurrentImageID returns the ID of the image currently selected for display.
// The current image is always the first entry in ordered_ids.
func (r *RustFSDatabase) GetCurrentImageID(ctx context.Context) (string, error) {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return "", err
	}
	if len(rs.OrderedIDs) == 0 {
		return "", fmt.Errorf("no images")
	}
	return rs.OrderedIDs[0], nil
}

// GetCurrentImageURL returns the browser-facing URL for the given image ID and
// variant ("original" or "processed"), routed through the ingress.
func (r *RustFSDatabase) GetCurrentImageURL(_ context.Context, id, variant string) (string, error) {
	switch variant {
	case "processed":
		return r.imageBaseURL + "/" + id + "/processed.png", nil
	default:
		return r.imageBaseURL + "/" + id + "/original.png", nil
	}
}

// GetLastRotatedTime reads the last-rotated timestamp from rotation.json.
// Returns an error when the timestamp is not yet set (first reconcile).
func (r *RustFSDatabase) GetLastRotatedTime(ctx context.Context) (time.Time, error) {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if rs.LastRotated.IsZero() {
		return time.Time{}, fmt.Errorf("last-rotated key not set")
	}
	return rs.LastRotated, nil
}

// insertIDAfter inserts newID immediately after afterID in ids.
// If afterID is empty or not found, newID is appended.
func insertIDAfter(ids []string, newID, afterID string) []string {
	if afterID == "" {
		return append(ids, newID)
	}
	for i, id := range ids {
		if id == afterID {
			result := make([]string, len(ids)+1)
			copy(result, ids[:i+1])
			result[i+1] = newID
			copy(result[i+2:], ids[i+1:])
			return result
		}
	}
	return append(ids, newID)
}

// removeID returns a new slice with id removed.
func removeID(ids []string, id string) []string {
	result := make([]string, 0, len(ids))
	for _, v := range ids {
		if v != id {
			result = append(result, v)
		}
	}
	return result
}

// RotationStateClient is a lightweight S3-only client for reading and writing
// rotation.json. It is used by both the server (embedded in RustFSDatabase) and
// the operator (which cannot access the server's storage directly).
type RotationStateClient struct {
	s3 *s3Client
}

// NewRotationStateClient creates a client that reads and writes rotation.json
// in the given RustFS bucket.
func NewRotationStateClient(endpoint, bucket, accessKey, secretKey string) (*RotationStateClient, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("rustfs: endpoint must not be empty")
	}
	if bucket == "" {
		return nil, fmt.Errorf("rustfs: bucket must not be empty")
	}
	return &RotationStateClient{s3: newS3Client(endpoint, bucket, accessKey, secretKey, "us-east-1")}, nil
}

func (c *RotationStateClient) getRotationState(ctx context.Context) (rotationState, error) {
	data, err := c.s3.GetObject(ctx, rotationStateKey)
	if err != nil {
		return rotationState{}, fmt.Errorf("s3: reading rotation state: %w", err)
	}
	if data == nil {
		return rotationState{}, nil
	}
	var rs rotationState
	if err := json.Unmarshal(data, &rs); err != nil {
		return rotationState{}, fmt.Errorf("s3: parsing rotation state: %w", err)
	}
	return rs, nil
}

func (c *RotationStateClient) putRotationState(ctx context.Context, rs rotationState) error {
	data, err := json.Marshal(rs)
	if err != nil {
		return fmt.Errorf("s3: marshalling rotation state: %w", err)
	}
	return c.s3.PutObject(ctx, rotationStateKey, "application/json", data)
}

// GetOrderedIDs returns the current ordered image ID list from rotation.json.
func (c *RotationStateClient) GetOrderedIDs(ctx context.Context) ([]string, error) {
	rs, err := c.getRotationState(ctx)
	if err != nil {
		return nil, err
	}
	return rs.OrderedIDs, nil
}

// GetLastRotatedTime returns the last-rotated timestamp from rotation.json.
// Returns an error when the timestamp is not yet set (first reconcile).
func (c *RotationStateClient) GetLastRotatedTime(ctx context.Context) (time.Time, error) {
	rs, err := c.getRotationState(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if rs.LastRotated.IsZero() {
		return time.Time{}, fmt.Errorf("last-rotated key not set")
	}
	return rs.LastRotated, nil
}

// SetRotationKeys writes last_rotated and the ordered ID list to rotation.json.
// The current image is always ordered_ids[0].
func (c *RotationStateClient) SetRotationKeys(ctx context.Context, rotatedAt time.Time, orderedIDs []string) error {
	rs, err := c.getRotationState(ctx)
	if err != nil {
		return err
	}
	rs.LastRotated = rotatedAt.UTC()
	rs.OrderedIDs = orderedIDs
	return c.putRotationState(ctx, rs)
}
