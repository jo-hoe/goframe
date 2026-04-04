package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jo-hoe/goframe/internal/database"
	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// redisRequeueOnNotReady is the requeue interval when Redis is not yet reachable.
const redisRequeueOnNotReady = 15 * time.Second

// reconcileRotation performs timezone-aware midnight rotation and writes the
// resulting current image ID to the Redis key that the server reads.
// It returns the duration until the next midnight (for RequeueAfter).
func (r *GoFrameReconciler) reconcileRotation(ctx context.Context, gf *goframev1alpha1.GoFrame) (time.Duration, error) {
	logger := log.FromContext(ctx)

	tz := gf.Spec.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		logger.Info("invalid timezone in spec, falling back to UTC", "tz", tz)
		loc = time.UTC
	}

	now := time.Now().In(loc)
	nextMidnight := durationUntilNextMidnight(now, loc)

	// Connect to Redis for this CR instance.
	db, err := database.NewRedisDatabase(gf.Spec.Redis.Address, "", 0, gf.Name)
	if err != nil {
		// Redis may not be ready yet (e.g. first reconcile). Requeue quickly.
		logger.Info("redis not yet reachable, requeuing", "addr", gf.Spec.Redis.Address, "err", err)
		return redisRequeueOnNotReady, nil
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			slog.Warn("reconcileRotation: failed to close Redis connection", "err", cerr)
		}
	}()

	// Advance the rotation by elapsed days.
	if err := advanceRotation(ctx, db, now, gf); err != nil {
		return nextMidnight, fmt.Errorf("advancing rotation: %w", err)
	}

	// Read the current first image.
	currentID, err := db.GetCurrentImageID(ctx)
	if err != nil {
		// No images yet — nothing to rotate.
		logger.Info("no images in database yet, skipping rotation key write")
		return nextMidnight, nil
	}

	// Update status with current image and rotation time.
	now2 := metav1.Now()
	gf.Status.CurrentImageID = currentID
	gf.Status.LastRotationTime = &now2

	if err := r.Status().Update(ctx, gf); err != nil {
		logger.Error(err, "failed to update GoFrame status after rotation")
		// Non-fatal.
	}

	logger.Info("rotation reconciled", "currentImageID", currentID, "nextIn", nextMidnight)
	return nextMidnight, nil
}

// advanceRotation checks if any days have elapsed since the last-rotated key in Redis and,
// if so, rotates the image order by the appropriate number of positions.
func advanceRotation(ctx context.Context, db database.DatabaseService, now time.Time, gf *goframev1alpha1.GoFrame) error {
	images, err := db.GetImages(ctx, "id")
	if err != nil || len(images) == 0 {
		return nil
	}

	ids := make([]string, 0, len(images))
	for _, img := range images {
		ids = append(ids, img.ID)
	}

	rdb, ok := db.(*database.RedisDatabase)
	if !ok {
		return nil
	}

	lastRotated, err := rdb.GetLastRotatedTime(ctx)
	if err != nil {
		// Key not yet set — first reconcile. Initialise and return.
		return rdb.SetRotationKeys(ctx, ids[0], now)
	}

	tz := gf.Spec.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	todayMid := dayStart(now, loc)
	lastMid := dayStart(lastRotated, loc)

	if !todayMid.After(lastMid) {
		return nil // Same day — no rotation needed.
	}

	days := int(todayMid.Sub(lastMid).Hours() / 24.0)
	if days > 0 {
		k := days % len(ids)
		newOrder := append([]string{}, ids[k:]...)
		newOrder = append(newOrder, ids[:k]...)
		if err := db.UpdateRanks(ctx, newOrder); err != nil {
			return fmt.Errorf("updating ranks: %w", err)
		}
		ids = newOrder
	}

	return rdb.SetRotationKeys(ctx, ids[0], now)
}

// durationUntilNextMidnight returns how long until 00:00 in the given location.
func durationUntilNextMidnight(now time.Time, loc *time.Location) time.Duration {
	t := now.In(loc)
	next := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
	d := next.Sub(now)
	if d <= 0 {
		d = 24 * time.Hour
	}
	return d
}

// dayStart returns 00:00 in the given location for the day of t.
func dayStart(t time.Time, loc *time.Location) time.Time {
	tt := t.In(loc)
	return time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, loc)
}

// updateStatus reconciles the Ready conditions on the GoFrame status.
func (r *GoFrameReconciler) updateStatus(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	serverDeploy, err := r.getDeploymentReadyReplicas(ctx, serverName(gf), gf.Namespace)
	if err != nil {
		ctrl.Log.Info("could not check server deployment", "err", err)
	}
	gf.Status.ServerReady = serverDeploy > 0

	return r.Status().Update(ctx, gf)
}
