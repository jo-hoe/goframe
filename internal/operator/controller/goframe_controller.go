// Package controller implements the GoFrame Kubernetes operator reconciler.
package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

// GoFrameReconciler reconciles GoFrame custom resources.
//
// +kubebuilder:rbac:groups=goframe.io,resources=goframes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goframe.io,resources=goframes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goframe.io,resources=goframes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
type GoFrameReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager registers the reconciler with the controller-runtime manager
// and declares ownership over the resources it manages.
func (r *GoFrameReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Enqueue the GoFrame that references a Secret when that Secret changes.
	secretToGoFrame := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		gfList := &goframev1alpha1.GoFrameList{}
		if err := r.List(ctx, gfList, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}
		var reqs []reconcile.Request
		for _, gf := range gfList.Items {
			if gf.Spec.ObjectStorage.SecretRef == obj.GetName() {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: gf.Name, Namespace: gf.Namespace},
				})
			}
		}
		return reqs
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&goframev1alpha1.GoFrame{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchv1.CronJob{}).
		Watches(&corev1.Secret{}, secretToGoFrame).
		Complete(r)
}

// Reconcile is called whenever a GoFrame CR (or an owned resource) changes.
// It drives the cluster toward the desired state described by the CR.
func (r *GoFrameReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	gf := &goframev1alpha1.GoFrame{}
	if err := r.Get(ctx, req.NamespacedName, gf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconciling GoFrame", "name", gf.Name, "namespace", gf.Namespace)

	if err := r.reconcileServer(ctx, gf); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcileServer: %w", err)
	}

	if err := r.reconcileCronJobs(ctx, gf); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcileCronJobs: %w", err)
	}

	requeueAfter, err := r.reconcileRotation(ctx, gf)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcileRotation: %w", err)
	}

	if err := r.updateStatus(ctx, gf); err != nil {
		logger.Error(err, "failed to update status")
		// Non-fatal: requeue and retry.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
