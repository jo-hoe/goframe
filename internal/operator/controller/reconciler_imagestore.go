package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

const (
	imageStorePort         int32 = 5000
	imageStorePathPrefix         = "/gallery"
	imageStoreDataPath           = "/data"
	imageStoreBinary             = "dufs"
	imageStoreDefaultImage       = "sigoden/dufs"
	imageStoreDefaultSize        = "5Gi"
)

// imageStoreName returns the name for image store resources owned by a GoFrame CR.
func imageStoreName(gf *goframev1alpha1.GoFrame) string {
	return gf.Name + "-imagestore"
}

// reconcileImageStore ensures the image store Deployment, Service, and PVC match
// the CR spec. When the image store is disabled it deletes any owned resources.
func (r *GoFrameReconciler) reconcileImageStore(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	if !gf.Spec.ImageStore.Enabled {
		return r.deleteImageStore(ctx, gf)
	}
	if err := r.reconcileImageStorePVC(ctx, gf); err != nil {
		return err
	}
	credHash, err := r.imageStoreCredentialsHash(ctx, gf)
	if err != nil {
		return err
	}
	if err := r.reconcileImageStoreDeployment(ctx, gf, credHash); err != nil {
		return err
	}
	return r.reconcileImageStoreService(ctx, gf)
}

// imageStoreCredentialsHash returns a hash of the image store credentials so the
// Deployment rolls when the Secret changes. Returns "" when no Secret is set.
func (r *GoFrameReconciler) imageStoreCredentialsHash(ctx context.Context, gf *goframev1alpha1.GoFrame) (string, error) {
	ref := gf.Spec.ImageStore.SecretRef
	if ref == "" {
		return "", nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref, Namespace: gf.Namespace}, secret); err != nil {
		return "", fmt.Errorf("reading image store secret %q: %w", ref, err)
	}
	data := append(secret.Data["username"], secret.Data["password"]...)
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func (r *GoFrameReconciler) reconcileImageStorePVC(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	size := gf.Spec.ImageStore.Persistence.Size
	if size == "" {
		size = imageStoreDefaultSize
	}
	qty, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("parsing image store persistence size %q: %w", size, err)
	}
	accessMode := corev1.PersistentVolumeAccessMode(gf.Spec.ImageStore.Persistence.AccessMode)
	if accessMode == "" {
		accessMode = corev1.ReadWriteOnce
	}

	desired := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageStoreName(gf),
			Namespace: gf.Namespace,
			Labels:    imageStoreLabels(gf),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: qty},
			},
		},
	}
	if sc := gf.Spec.ImageStore.Persistence.StorageClass; sc != "" {
		desired.Spec.StorageClassName = &sc
	}
	if err := ctrl.SetControllerReference(gf, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	// A PVC's spec is largely immutable after creation; we create it once and
	// otherwise leave it untouched (resizing is a deliberate, separate operation).
	return err
}

// buildImageStoreDeployment renders the desired hardened dufs Deployment. It is
// pure (no client calls) so it can be unit-tested.
func buildImageStoreDeployment(gf *goframev1alpha1.GoFrame, credHash string) *appsv1.Deployment {
	spec := gf.Spec.ImageStore
	replicas := int32(1)
	if spec.Replicas != nil {
		replicas = *spec.Replicas
	}
	port := spec.Port
	if port == 0 {
		port = imageStorePort
	}
	pathPrefix := spec.PathPrefix
	if pathPrefix == "" {
		pathPrefix = imageStorePathPrefix
	}

	container := corev1.Container{
		Name:            imageStoreBinary,
		Image:           imageStoreImageRef(gf),
		ImagePullPolicy: imagePullPolicy(spec.Image.PullPolicy),
		Args:            imageStoreArgs(spec.SecretRef),
		Env:             imageStoreEnvVars(gf, port, pathPrefix),
		Ports: []corev1.ContainerPort{
			{ContainerPort: port, Protocol: corev1.ProtocolTCP},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data", MountPath: imageStoreDataPath},
		},
		ReadinessProbe:  imageStoreProbe(port, pathPrefix, 5, 10),
		LivenessProbe:   imageStoreProbe(port, pathPrefix, 15, 20),
		SecurityContext: serverContainerSecurityContext(),
	}
	if res := spec.Resources; res != nil {
		container.Resources = *res
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageStoreName(gf),
			Namespace: gf.Namespace,
			Labels:    imageStoreLabels(gf),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			// dufs serves a single ReadWriteOnce PVC; Recreate avoids two pods
			// contending for the same volume during a rollout.
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: imageStoreLabels(gf)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      imageStoreLabels(gf),
					Annotations: map[string]string{"checksum/credentials": credHash},
				},
				Spec: corev1.PodSpec{
					SecurityContext: serverPodSecurityContext(),
					Containers:      []corev1.Container{container},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: imageStoreName(gf),
								},
							},
						},
					},
				},
			},
		},
	}
}

// imageStoreArgs builds the dufs argument list. The --auth rule is only added
// when a credentials Secret is configured; it references env vars populated from
// that Secret so the plaintext credential never appears in the pod spec. The
// "@/" suffix grants anonymous read so image URLs resolve without auth while
// uploads/deletes require the credential.
func imageStoreArgs(secretRef string) []string {
	args := []string{imageStoreDataPath}
	if secretRef != "" {
		args = append(args, "--auth", "$(IMAGESTORE_USERNAME):$(IMAGESTORE_PASSWORD)@/:rw|@/")
	}
	return args
}

// imageStoreEnvVars returns the dufs configuration env vars. Credentials are
// sourced from the Secret only when a SecretRef is set.
func imageStoreEnvVars(gf *goframev1alpha1.GoFrame, port int32, pathPrefix string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "DUFS_SERVE_PATH", Value: imageStoreDataPath},
		{Name: "DUFS_BIND", Value: "0.0.0.0"},
		{Name: "DUFS_PORT", Value: strconv.Itoa(int(port))},
		{Name: "DUFS_PATH_PREFIX", Value: pathPrefix},
		{Name: "DUFS_ALLOW_UPLOAD", Value: "true"},
		{Name: "DUFS_ALLOW_DELETE", Value: "true"},
	}
	if ref := gf.Spec.ImageStore.SecretRef; ref != "" {
		env = append(env,
			corev1.EnvVar{
				Name: "IMAGESTORE_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: ref},
						Key:                  "username",
					},
				},
			},
			corev1.EnvVar{
				Name: "IMAGESTORE_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: ref},
						Key:                  "password",
					},
				},
			},
		)
	}
	return env
}

// imageStoreProbe builds an HTTP GET probe against the dufs health endpoint,
// which dufs serves under its configured path prefix.
func imageStoreProbe(port int32, pathPrefix string, initialDelay, period int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: pathPrefix + "/__dufs__/health",
				Port: intstr.FromInt32(port),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
	}
}

func (r *GoFrameReconciler) reconcileImageStoreDeployment(ctx context.Context, gf *goframev1alpha1.GoFrame, credHash string) error {
	desired := buildImageStoreDeployment(gf, credHash)
	if err := ctrl.SetControllerReference(gf, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existingContainer := &existing.Spec.Template.Spec.Containers[0]
	desiredContainer := &desired.Spec.Template.Spec.Containers[0]
	needsUpdate := existingContainer.Image != desiredContainer.Image ||
		existingContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy ||
		existing.Spec.Template.Annotations["checksum/credentials"] != credHash ||
		!equalInt32Ptr(existing.Spec.Replicas, desired.Spec.Replicas) ||
		existing.Spec.Strategy.Type != desired.Spec.Strategy.Type ||
		!equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers)
	if needsUpdate {
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.Strategy = desired.Spec.Strategy
		existing.Spec.Template.Spec = desired.Spec.Template.Spec
		if existing.Spec.Template.Annotations == nil {
			existing.Spec.Template.Annotations = map[string]string{}
		}
		existing.Spec.Template.Annotations["checksum/credentials"] = credHash
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *GoFrameReconciler) reconcileImageStoreService(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	port := gf.Spec.ImageStore.Port
	if port == 0 {
		port = imageStorePort
	}
	svcType := corev1.ServiceType(gf.Spec.ImageStore.ServiceType)
	if svcType == "" {
		svcType = corev1.ServiceTypeClusterIP
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageStoreName(gf),
			Namespace: gf.Namespace,
			Labels:    imageStoreLabels(gf),
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: imageStoreLabels(gf),
			Ports: []corev1.ServicePort{
				{Port: port, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	if err := ctrl.SetControllerReference(gf, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if existing.Spec.Type != desired.Spec.Type ||
		!equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) {
		existing.Spec.Type = desired.Spec.Type
		existing.Spec.Ports = desired.Spec.Ports
		return r.Update(ctx, existing)
	}
	return nil
}

// deleteImageStore removes all image store resources (Deployment, Service, PVC)
// owned by the CR when the image store is disabled. The PVC is intentionally
// retained to avoid data loss when toggling the feature off.
func (r *GoFrameReconciler) deleteImageStore(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	name := types.NamespacedName{Name: imageStoreName(gf), Namespace: gf.Namespace}
	if err := r.deleteIfExists(ctx, &appsv1.Deployment{}, name); err != nil {
		return err
	}
	return r.deleteIfExists(ctx, &corev1.Service{}, name)
}

// deleteIfExists deletes the named object, ignoring a not-found result.
func (r *GoFrameReconciler) deleteIfExists(ctx context.Context, obj client.Object, name types.NamespacedName) error {
	if err := r.Get(ctx, name, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func imageStoreLabels(gf *goframev1alpha1.GoFrame) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "goframe",
		"app.kubernetes.io/instance":  gf.Name,
		"app.kubernetes.io/component": "imagestore",
	}
}

func imageStoreImageRef(gf *goframev1alpha1.GoFrame) string {
	img := gf.Spec.ImageStore.Image
	repo := img.Repository
	if repo == "" {
		repo = imageStoreDefaultImage
	}
	tag := img.Tag
	if tag == "" {
		tag = "latest"
	}
	return repo + ":" + tag
}
