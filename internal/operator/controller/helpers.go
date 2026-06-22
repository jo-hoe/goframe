package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
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

// readObjectStorageCredentials reads accessKey and secretKey from the Secret named in
// gf.Spec.ObjectStorage.SecretRef. Returns empty strings (no error) when SecretRef is empty.
func (r *GoFrameReconciler) readObjectStorageCredentials(ctx context.Context, gf *goframev1alpha1.GoFrame) (accessKey, secretKey string, err error) {
	ref := gf.Spec.ObjectStorage.SecretRef
	if ref == "" {
		return "", "", nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref, Namespace: gf.Namespace}, secret); err != nil {
		return "", "", fmt.Errorf("reading object-storage secret %q: %w", ref, err)
	}
	return string(secret.Data["accessKey"]), string(secret.Data["secretKey"]), nil
}
