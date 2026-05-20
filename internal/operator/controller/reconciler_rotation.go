package controller

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jo-hoe/goframe/internal/database"
	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rustfsRequeueOnNotReady is the requeue interval when RustFS is not yet reachable.
const rustfsRequeueOnNotReady = 15 * time.Second

// reconcileRotation performs timezone-aware midnight rotation and writes the
// resulting ordered_ids list to rotation.json in RustFS, which the server also reads.
// The current image is always ordered_ids[0].
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

	bucket := gf.Spec.RustFS.Bucket
	if bucket == "" {
		bucket = gf.Name
	}

	accessKey, secretKey, err := r.readRustFSCredentials(ctx, gf)
	if err != nil {
		logger.Info("could not read RustFS credentials, requeuing", "err", err)
		return rustfsRequeueOnNotReady, nil
	}

	rc, err := database.NewRotationStateClient(gf.Spec.RustFS.Endpoint, bucket, accessKey, secretKey)
	if err != nil {
		logger.Info("could not create rotation state client, requeuing", "endpoint", gf.Spec.RustFS.Endpoint, "err", err)
		return rustfsRequeueOnNotReady, nil
	}

	if err := advanceRotation(ctx, rc, now, gf); err != nil {
		return nextMidnight, fmt.Errorf("advancing rotation: %w", err)
	}

	ids, err := rc.GetOrderedIDs(ctx)
	if err != nil || len(ids) == 0 {
		logger.Info("no images in rotation state yet, skipping status update")
		return nextMidnight, nil
	}

	currentID := ids[0]

	now2 := metav1.Now()
	gf.Status.CurrentImageID = currentID
	gf.Status.LastRotationTime = &now2

	if err := r.Status().Update(ctx, gf); err != nil {
		logger.Error(err, "failed to update GoFrame status after rotation")
	}

	logger.Info("rotation reconciled", "currentImageID", currentID, "nextIn", nextMidnight)
	return nextMidnight, nil
}

// advanceRotation checks if any days have elapsed since the last-rotated key and,
// if so, rotates the image order by the appropriate number of positions.
func advanceRotation(ctx context.Context, rc *database.RotationStateClient, now time.Time, gf *goframev1alpha1.GoFrame) error {
	ids, err := rc.GetOrderedIDs(ctx)
	if err != nil || len(ids) == 0 {
		return nil
	}

	lastRotated, err := rc.GetLastRotatedTime(ctx)
	if err != nil {
		// Key not yet set — first reconcile. Initialise and return.
		return rc.SetRotationKeys(ctx, now, ids)
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
		ids = newOrder
	}

	return rc.SetRotationKeys(ctx, now, ids)
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

