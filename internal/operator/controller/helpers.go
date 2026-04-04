package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// getDeploymentReadyReplicas returns the number of ready replicas for a named Deployment.
// Returns 0 if the Deployment doesn't exist.
func (r *GoFrameReconciler) getDeploymentReadyReplicas(ctx context.Context, name, namespace string) (int32, error) {
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploy); err != nil {
		return 0, nil
	}
	return deploy.Status.ReadyReplicas, nil
}
