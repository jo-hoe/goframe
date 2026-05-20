package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	// modernc pure-Go SQLite driver; blank import registers the "sqlite" driver.
	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

// initSchema creates the images table if it does not exist and enables WAL mode
// for Litestream compatibility. Rotation state is stored in RustFS (rotation.json),
// not in SQLite.
func initSchema(db *sql.DB) error {
	_, err := db.Exec(`PRAGMA journal_mode=WAL;`)
	if err != nil {
		return fmt.Errorf("sqlite: enabling WAL mode: %w", err)
	}
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS images (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    rank        REAL NOT NULL,
    source      TEXT NOT NULL DEFAULT ''
);
`)
	return err
}

// openSQLite opens (or creates) the SQLite database at path.
func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening %q: %w", path, err)
	}
	// SQLite does not support concurrent writers; serialise via a single connection.
	db.SetMaxOpenConns(1)
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: initialising schema: %w", err)
	}
	return db, nil
}

// sqliteNextRank computes the score at which to insert a new image.
func sqliteNextRank(ctx context.Context, tx *sql.Tx, afterID string) (float64, error) {
	if afterID == "" {
		var max sql.NullFloat64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(rank) FROM images`).Scan(&max); err != nil {
			return 0, fmt.Errorf("sqlite: reading max rank: %w", err)
		}
		if !max.Valid {
			return 1, nil
		}
		return max.Float64 + 1, nil
	}

	var afterRank float64
	err := tx.QueryRowContext(ctx, `SELECT rank FROM images WHERE id = ?`, afterID).Scan(&afterRank)
	if err != nil {
		return 0, fmt.Errorf("sqlite: reading rank for afterID %q: %w", afterID, err)
	}

	var nextRank sql.NullFloat64
	err = tx.QueryRowContext(ctx,
		`SELECT rank FROM images WHERE rank > ? ORDER BY rank ASC LIMIT 1`, afterRank,
	).Scan(&nextRank)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("sqlite: reading successor rank: %w", err)
	}
	if !nextRank.Valid {
		return afterRank + 0.5, nil
	}
	return afterRank + (nextRank.Float64-afterRank)/2, nil
}

const rotationStateKey = "rotation.json"

// rotationState is the JSON structure stored as rotation.json in RustFS.
// It is the single source of truth for rotation shared between the server and the operator.
// The current image is always OrderedIDs[0]; there is no separate current_id pointer.
type rotationState struct {
	LastRotated time.Time `json:"last_rotated"`
	OrderedIDs  []string  `json:"ordered_ids"`
}

// RustFSDatabase implements DatabaseService using RustFS (S3-compatible) for
// image blobs and rotation state, and SQLite for ordered metadata.
type RustFSDatabase struct {
	db           *sql.DB
	s3           *s3Client
	bucket       string
	imageBaseURL string
}

// NewRustFSDatabase opens the SQLite database at dbPath and connects to the
// RustFS endpoint. bucket is the S3 bucket name used for image objects.
// imageBaseURL is the browser-facing URL prefix for image assets (e.g. "/images").
func NewRustFSDatabase(endpoint, bucket, accessKey, secretKey, region, dbPath, imageBaseURL string) (DatabaseService, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("rustfs: endpoint must not be empty")
	}
	if bucket == "" {
		return nil, fmt.Errorf("rustfs: bucket must not be empty")
	}
	if dbPath == "" {
		dbPath = "/data/goframe.db"
	}
	if imageBaseURL == "" {
		imageBaseURL = "/images"
	}

	sqlDB, err := openSQLite(dbPath)
	if err != nil {
		return nil, err
	}

	if region == "" {
		region = "us-east-1"
	}
	s3 := newS3Client(endpoint, bucket, accessKey, secretKey, region)

	if err := s3.EnsureBucket(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("rustfs: ensuring bucket %q exists: %w", bucket, err)
	}

	if err := s3.SetPublicReadPolicy(context.Background(), "images/*"); err != nil {
		slog.Warn("rustfs: could not set public read policy (anonymous image access may not work via ingress)", "error", err)
	}

	return &RustFSDatabase{db: sqlDB, s3: s3, bucket: bucket, imageBaseURL: imageBaseURL}, nil
}

// Close shuts down the SQLite connection.
func (r *RustFSDatabase) Close() error {
	return r.db.Close()
}

// imageOriginalKey returns the S3 object key for the original image.
func imageOriginalKey(id string) string { return "images/" + id + "/original.png" }

// imageProcessedKey returns the S3 object key for the processed image.
func imageProcessedKey(id string) string { return "images/" + id + "/processed.png" }

// getRotationState reads rotation.json from RustFS.
// Returns an empty state (not an error) when the object does not yet exist.
func (r *RustFSDatabase) getRotationState(ctx context.Context) (rotationState, error) {
	data, err := r.s3.GetObject(ctx, rotationStateKey)
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

// putRotationState writes the rotation state to rotation.json in RustFS.
func (r *RustFSDatabase) putRotationState(ctx context.Context, rs rotationState) error {
	data, err := json.Marshal(rs)
	if err != nil {
		return fmt.Errorf("s3: marshalling rotation state: %w", err)
	}
	return r.s3.PutObject(ctx, rotationStateKey, "application/json", data)
}

// CreateImage stores original and processed blobs in RustFS and records
// metadata in SQLite. When afterID is empty the image is appended; when set
// it is inserted immediately after that image using midpoint scoring.
func (r *RustFSDatabase) CreateImage(ctx context.Context, original []byte, processed []byte, createdAt time.Time, source string, afterID string) (string, error) {
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

	// Upload blobs to RustFS first. If either upload fails we never write the
	// SQLite row, keeping storage and metadata in sync.
	if err := r.s3.PutObject(ctx, imageOriginalKey(id), "image/png", original); err != nil {
		return "", fmt.Errorf("rustfs: uploading original for %s: %w", id, err)
	}
	if err := r.s3.PutObject(ctx, imageProcessedKey(id), "image/png", processed); err != nil {
		_ = r.s3.DeleteObject(ctx, imageOriginalKey(id))
		return "", fmt.Errorf("rustfs: uploading processed for %s: %w", id, err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rank, err := sqliteNextRank(ctx, tx, afterID)
	if err != nil {
		return "", err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO images (id, created_at, rank, source) VALUES (?, ?, ?, ?)`,
		id, createdAt.UTC().Format(time.RFC3339), rank, source,
	)
	if err != nil {
		return "", fmt.Errorf("sqlite: inserting image row: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("sqlite: committing image row: %w", err)
	}

	// Update rotation state: append the new ID at the correct position.
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return id, fmt.Errorf("rustfs: reading rotation state after create: %w", err)
	}
	rs.OrderedIDs = insertIDAfter(rs.OrderedIDs, id, afterID)
	if err := r.putRotationState(ctx, rs); err != nil {
		return id, fmt.Errorf("rustfs: updating rotation state after create: %w", err)
	}

	return id, nil
}

// insertIDAfter inserts newID into ids immediately after afterID.
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

// GetImages returns images with only the specified fields populated.
// Image blobs (OriginalImage/ProcessedImage) are never fetched from RustFS
// here — callers should use GetCurrentImageURL to get redirect URLs instead.
// If no fields are provided all metadata fields are returned.
func (r *RustFSDatabase) GetImages(ctx context.Context, fields ...string) ([]*Image, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, created_at, source FROM images ORDER BY rank ASC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying images: %w", err)
	}
	defer func() { _ = rows.Close() }()

	wantAll := len(fields) == 0
	want := func(f string) bool {
		return wantAll || slices.Contains(fields, f)
	}

	var images []*Image
	for rows.Next() {
		var id, createdAtStr, source string
		if err := rows.Scan(&id, &createdAtStr, &source); err != nil {
			return nil, fmt.Errorf("sqlite: scanning image row: %w", err)
		}
		img := &Image{}
		if want("id") {
			img.ID = id
		}
		if want("created_at") {
			if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				img.CreatedAt = t
			}
		}
		if want("source") {
			img.Source = source
		}
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating images: %w", err)
	}
	return images, nil
}

// GetImagesBySource returns all images with the given source label, ordered by rank.
func (r *RustFSDatabase) GetImagesBySource(ctx context.Context, source string) ([]*Image, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM images WHERE source = ? ORDER BY rank ASC`, source)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying images by source: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var images []*Image
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: scanning image id: %w", err)
		}
		images = append(images, &Image{ID: id, Source: source})
	}
	return images, rows.Err()
}

// DeleteImage removes the image blobs from RustFS and the metadata row from SQLite.
// It also removes the ID from the rotation state ordered list and advances
// current_id if needed.
func (r *RustFSDatabase) DeleteImage(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM images WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: deleting image row %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: committing delete: %w", err)
	}

	// Update rotation state: remove the deleted ID and advance current if needed.
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return fmt.Errorf("rustfs: reading rotation state after delete: %w", err)
	}
	rs.OrderedIDs = removeID(rs.OrderedIDs, id)
	if err := r.putRotationState(ctx, rs); err != nil {
		return fmt.Errorf("rustfs: updating rotation state after delete: %w", err)
	}

	// Remove blobs from RustFS after state is committed.
	_ = r.s3.DeleteObject(ctx, imageOriginalKey(id))
	_ = r.s3.DeleteObject(ctx, imageProcessedKey(id))
	return nil
}

// removeID returns a new slice with id removed.
func removeID(ids []string, id string) []string {
	result := ids[:0:len(ids)]
	for _, v := range ids {
		if v != id {
			result = append(result, v)
		}
	}
	return result
}

// GetImageByID returns a single image's metadata. Blobs are not fetched; use
// GetCurrentImageURL to get redirect URLs.
func (r *RustFSDatabase) GetImageByID(ctx context.Context, id string) (*Image, error) {
	var createdAtStr, source string
	err := r.db.QueryRowContext(ctx,
		`SELECT created_at, source FROM images WHERE id = ?`, id,
	).Scan(&createdAtStr, &source)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("image not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: fetching image %s: %w", id, err)
	}

	img := &Image{ID: id, Source: source}
	if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		img.CreatedAt = t
	}
	return img, nil
}

// UpdateRanks resets the rank column to 1..N in the given order, atomically,
// and updates the ordered_ids list in rotation.json.
func (r *RustFSDatabase) UpdateRanks(ctx context.Context, order []string) error {
	if len(order) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, id := range order {
		if _, err := tx.ExecContext(ctx,
			`UPDATE images SET rank = ? WHERE id = ?`, float64(i+1), id); err != nil {
			return fmt.Errorf("sqlite: updating rank for %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	rs, err := r.getRotationState(ctx)
	if err != nil {
		return fmt.Errorf("rustfs: reading rotation state during UpdateRanks: %w", err)
	}
	rs.OrderedIDs = order
	return r.putRotationState(ctx, rs)
}

// GetCurrentImageID returns the ID of the image currently selected for display.
// The current image is always the first entry in ordered_ids.
func (r *RustFSDatabase) GetCurrentImageID(ctx context.Context) (string, error) {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return "", err
	}
	if len(rs.OrderedIDs) > 0 {
		return rs.OrderedIDs[0], nil
	}
	return "", fmt.Errorf("no images")
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
// Returns an error when the key is not yet set (first reconcile).
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

// SetRotationKeys atomically writes the last-rotated timestamp
// to rotation.json in RustFS. The current image is always ordered_ids[0].
func (r *RustFSDatabase) SetRotationKeys(ctx context.Context, rotatedAt time.Time) error {
	rs, err := r.getRotationState(ctx)
	if err != nil {
		return err
	}
	rs.LastRotated = rotatedAt.UTC()
	return r.putRotationState(ctx, rs)
}

// RotationStateClient is a lightweight S3-only client for reading and writing
// rotation state. It does not open a SQLite database and is suitable for use
// in the operator, which cannot access the server's PVC-backed SQLite file.
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
	s3 := newS3Client(endpoint, bucket, accessKey, secretKey, "us-east-1")
	return &RotationStateClient{s3: s3}, nil
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
// Returns an error when the key is not yet set (first reconcile).
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

// SetRotationKeys writes last_rotated and the new ordered ID list
// to rotation.json in a single PUT. The current image is always ordered_ids[0].
func (c *RotationStateClient) SetRotationKeys(ctx context.Context, rotatedAt time.Time, orderedIDs []string) error {
	rs := rotationState{
		LastRotated: rotatedAt.UTC(),
		OrderedIDs:  orderedIDs,
	}
	return c.putRotationState(ctx, rs)
}
