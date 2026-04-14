package database

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewDatabaseWithNamespace constructs a DatabaseService with a namespace (required for Redis).
func NewDatabaseWithNamespace(databaseType, connectionString, namespace string) (DatabaseService, error) {
	switch databaseType {
	case "redis":
		return NewRedisDatabase(connectionString, "", 0, namespace)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", databaseType)
	}
}

// redisImageFields are the hash field names stored per image.
const (
	fieldID             = "id"
	fieldCreatedAt      = "created_at"
	fieldOriginalImage  = "original"
	fieldProcessedImage = "processed"
	fieldSource         = "source"
)

// RedisDatabase implements DatabaseService using Redis.
// Image blobs are stored as base64-encoded hash fields.
// Ordering is maintained via a sorted set with sequential integer scores.
// The current image ID for rotation is read from a key managed by the operator.
type RedisDatabase struct {
	client    *redis.Client
	namespace string
}

// NewRedisDatabase creates a new RedisDatabase connected to the given address.
// namespace should equal the GoFrame CR name to isolate key spaces.
func NewRedisDatabase(addr, password string, dbIndex int, namespace string) (DatabaseService, error) {
	if namespace == "" {
		return nil, fmt.Errorf("redis namespace must not be empty")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbIndex,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return &RedisDatabase{client: client, namespace: namespace}, nil
}

// Close shuts down the Redis client.
func (r *RedisDatabase) Close() error {
	return r.client.Close()
}

// computeInsertScore returns the sorted-set score at which a new image should
// be inserted. It must be called from within a WATCH transaction so that the
// read is protected against concurrent modifications.
// When afterID is empty the score is appended after the current last entry.
// When afterID is set the score is the midpoint between afterID and its successor.
func computeInsertScore(ctx context.Context, tx *redis.Tx, orderKey, afterID string) (float64, error) {
	if afterID == "" {
		card, err := tx.ZCard(ctx, orderKey).Result()
		if err != nil {
			return 0, fmt.Errorf("getting order set cardinality: %w", err)
		}
		return float64(card + 1), nil
	}

	s, err := tx.ZScore(ctx, orderKey, afterID).Result()
	if err != nil {
		return 0, fmt.Errorf("getting score for afterID %q: %w", afterID, err)
	}
	nexts, err := tx.ZRangeByScoreWithScores(ctx, orderKey, &redis.ZRangeBy{
		Min:    fmt.Sprintf("(%g", s),
		Max:    "+inf",
		Offset: 0,
		Count:  1,
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("finding successor score: %w", err)
	}
	if len(nexts) == 0 {
		return s + 0.5, nil
	}
	return s + (nexts[0].Score-s)/2, nil
}

// writeImageTx queues the hash write and sorted-set insertion into pipe.
func writeImageTx(ctx context.Context, pipe redis.Pipeliner, hashKey, orderKey string, score float64, id string, original, processed []byte, createdAt time.Time, source string) {
	pipe.HSet(ctx, hashKey,
		fieldID, id,
		fieldCreatedAt, createdAt.Format(time.RFC3339),
		fieldOriginalImage, base64.StdEncoding.EncodeToString(original),
		fieldProcessedImage, base64.StdEncoding.EncodeToString(processed),
		fieldSource, source,
	)
	pipe.ZAdd(ctx, orderKey, redis.Z{Score: score, Member: id})
}

// CreateImage stores a new image in Redis.
// When afterID is empty the image is appended to the end of the queue.
// When afterID is set the image is inserted immediately after that image using
// a float64 score midpoint, so no other scores need to be rewritten.
// Score assignment and hash/sorted-set writes are protected by WATCH + optimistic
// retry to prevent duplicate scores under concurrent uploads.
func (r *RedisDatabase) CreateImage(ctx context.Context, original []byte, processed []byte, createdAt time.Time, source string, afterID string) (string, error) {
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

	orderKey := OrderSetKey(r.namespace)
	hashKey := ImageHashKey(r.namespace, id)

	const maxRetries = 25
	for range maxRetries {
		err = r.client.Watch(ctx, func(tx *redis.Tx) error {
			score, err := computeInsertScore(ctx, tx, orderKey, afterID)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				writeImageTx(ctx, pipe, hashKey, orderKey, score, id, original, processed, createdAt, source)
				return nil
			})
			return err
		}, orderKey)

		if err == nil || !errors.Is(err, redis.TxFailedErr) {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("creating image in redis: %w", err)
	}
	return id, nil
}

// GetImages returns images with only the specified fields populated.
// If no fields are provided, all fields are returned.
func (r *RedisDatabase) GetImages(ctx context.Context, fields ...string) ([]*Image, error) {
	ids, err := r.client.ZRange(ctx, OrderSetKey(r.namespace), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("listing image order: %w", err)
	}

	images := make([]*Image, 0, len(ids))
	for _, id := range ids {
		img, err := r.fetchImageFields(ctx, id, fields)
		if err != nil {
			return nil, err
		}
		if img != nil {
			images = append(images, img)
		}
	}
	return images, nil
}

// GetImagesBySource returns all images with the given source label, ordered by rank.
func (r *RedisDatabase) GetImagesBySource(ctx context.Context, source string) ([]*Image, error) {
	ids, err := r.client.ZRange(ctx, OrderSetKey(r.namespace), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("listing image order: %w", err)
	}

	var images []*Image
	for _, id := range ids {
		src, err := r.client.HGet(ctx, ImageHashKey(r.namespace, id), fieldSource).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("getting source for image %s: %w", id, err)
		}
		if src != source {
			continue
		}
		images = append(images, &Image{ID: id, Source: source})
	}
	return images, nil
}

// DeleteImage removes an image hash and its sorted-set entry.
// If the deleted image is the current rotation image, the rotation key is
// atomically advanced to the next image in rank order (or cleared if none remain).
func (r *RedisDatabase) DeleteImage(ctx context.Context, id string) error {
	// Check if this is the current image before deleting.
	currentID, err := r.client.Get(ctx, RotationCurrentIDKey(r.namespace)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("reading current image id: %w", err)
	}

	// Find the successor in rank order before removing from the set.
	var successorID string
	if currentID == id {
		ids, err := r.client.ZRange(ctx, OrderSetKey(r.namespace), 0, -1).Result()
		if err != nil {
			return fmt.Errorf("listing image order: %w", err)
		}
		for i, oid := range ids {
			if oid == id {
				if i+1 < len(ids) {
					successorID = ids[i+1]
				} else if i > 0 {
					successorID = ids[0] // wrap to first if deleting the last
				}
				break
			}
		}
	}

	pipe := r.client.TxPipeline()
	pipe.Del(ctx, ImageHashKey(r.namespace, id))
	pipe.ZRem(ctx, OrderSetKey(r.namespace), id)
	if currentID == id {
		if successorID != "" {
			pipe.Set(ctx, RotationCurrentIDKey(r.namespace), successorID, 0)
		} else {
			pipe.Del(ctx, RotationCurrentIDKey(r.namespace))
		}
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("deleting image %s: %w", id, err)
	}
	return nil
}

// GetImageByID fetches a single image by its ID.
func (r *RedisDatabase) GetImageByID(ctx context.Context, id string) (*Image, error) {
	return r.fetchImageFields(ctx, id, nil)
}

// UpdateRanks resets sorted-set scores to 1..N in the given order atomically.
func (r *RedisDatabase) UpdateRanks(ctx context.Context, order []string) error {
	if len(order) == 0 {
		return nil
	}

	orderKey := OrderSetKey(r.namespace)
	pipe := r.client.TxPipeline()
	for i, id := range order {
		score := float64(i + 1)
		pipe.ZAdd(ctx, orderKey, redis.Z{Score: score, Member: id})
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("updating ranks: %w", err)
	}
	return nil
}

// GetCurrentImageID reads the operator-managed rotation:current-id key.
// Falls back to the first image in order if the key has not been written yet.
func (r *RedisDatabase) GetCurrentImageID(ctx context.Context) (string, error) {
	id, err := r.client.Get(ctx, RotationCurrentIDKey(r.namespace)).Result()
	if err == redis.Nil {
		return r.firstImageID(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("reading current image id: %w", err)
	}
	return id, nil
}

// GetLastRotatedTime reads the last-rotated timestamp written by the operator.
// Returns a wrapped redis.Nil error if the key has never been written.
func (r *RedisDatabase) GetLastRotatedTime(ctx context.Context) (time.Time, error) {
	val, err := r.client.Get(ctx, RotationLastRotatedKey(r.namespace)).Result()
	if err == redis.Nil {
		return time.Time{}, fmt.Errorf("last-rotated key not set: %w", err)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("reading last-rotated: %w", err)
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing last-rotated timestamp %q: %w", val, err)
	}
	return t, nil
}

// SetRotationKeys atomically writes the current image ID and last-rotated timestamp.
func (r *RedisDatabase) SetRotationKeys(ctx context.Context, currentID string, rotatedAt time.Time) error {
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, RotationCurrentIDKey(r.namespace), currentID, 0)
	pipe.Set(ctx, RotationLastRotatedKey(r.namespace), rotatedAt.UTC().Format(time.RFC3339), 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("setting rotation keys: %w", err)
	}
	return nil
}

// firstImageID returns the ID of the image with the lowest score in the order set.
func (r *RedisDatabase) firstImageID(ctx context.Context) (string, error) {
	ids, err := r.client.ZRange(ctx, OrderSetKey(r.namespace), 0, 0).Result()
	if err != nil {
		return "", fmt.Errorf("listing first image: %w", err)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no images")
	}
	return ids[0], nil
}

// imageFieldMapping maps a logical field name to its Redis hash field name.
type imageFieldMapping struct {
	logical string
	redis   string
}

// allImageFields defines the complete ordered set of image hash fields.
var allImageFields = []imageFieldMapping{
	{"id", fieldID},
	{"created_at", fieldCreatedAt},
	{"original_image", fieldOriginalImage},
	{"processed_image", fieldProcessedImage},
	{"source", fieldSource},
}

// resolveRedisFields returns the Redis hash field names that correspond to the
// requested logical fields. If fields is empty all hash fields are returned.
func resolveRedisFields(fields []string) []string {
	if len(fields) == 0 {
		names := make([]string, len(allImageFields))
		for i, fm := range allImageFields {
			names[i] = fm.redis
		}
		return names
	}
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		want[f] = true
	}
	var names []string
	for _, fm := range allImageFields {
		if want[fm.logical] {
			names = append(names, fm.redis)
		}
	}
	return names
}

// hmgetToMap converts the positional HMGet result slice into a map keyed by
// Redis field name. Returns nil if every value is nil (key does not exist).
func hmgetToMap(redisFields []string, vals []any) map[string]string {
	result := make(map[string]string, len(redisFields))
	for i, rf := range redisFields {
		if vals[i] != nil {
			result[rf] = vals[i].(string)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// populateImage fills the fields of img from fieldVals according to which
// logical fields were requested. want returns true for any field that should
// be populated.
func populateImage(img *Image, fieldVals map[string]string, want func(string) bool) {
	if want("id") {
		img.ID = fieldVals[fieldID]
	}
	if want("created_at") {
		if t, err := time.Parse(time.RFC3339, fieldVals[fieldCreatedAt]); err == nil {
			img.CreatedAt = t
		}
	}
	if want("original_image") {
		img.OriginalImage = decodeBase64(fieldVals[fieldOriginalImage])
	}
	if want("processed_image") {
		img.ProcessedImage = decodeBase64(fieldVals[fieldProcessedImage])
	}
	if want("source") {
		img.Source = fieldVals[fieldSource]
	}
}

// fetchImageFields reads an image hash from Redis, populating only the requested fields.
// If fields is nil or empty, all fields are populated.
func (r *RedisDatabase) fetchImageFields(ctx context.Context, id string, fields []string) (*Image, error) {
	redisFields := resolveRedisFields(fields)

	vals, err := r.client.HMGet(ctx, ImageHashKey(r.namespace, id), redisFields...).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching image %s: %w", id, err)
	}

	fieldVals := hmgetToMap(redisFields, vals)
	if fieldVals == nil {
		return nil, nil // image not found
	}

	wantAll := len(fields) == 0
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[f] = true
	}
	want := func(f string) bool { return wantAll || fieldSet[f] }

	img := &Image{}
	populateImage(img, fieldVals, want)
	return img, nil
}

// decodeBase64 decodes a base64 string, returning nil on error.
func decodeBase64(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		log.Printf("redis: base64 decode error: %v", err)
		return nil
	}
	return b
}
