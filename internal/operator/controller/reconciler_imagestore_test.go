package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

func imageStoreGoFrame() *goframev1alpha1.GoFrame {
	gf := testGoFrame()
	gf.Spec.ImageStore = goframev1alpha1.ImageStoreSpec{
		Enabled:   true,
		SecretRef: "demo-imagestore-credentials",
	}
	return gf
}

func TestBuildImageStoreDeployment_Defaults(t *testing.T) {
	gf := imageStoreGoFrame()

	dep := buildImageStoreDeployment(gf, "credhash")

	if dep.Name != "demo-imagestore" {
		t.Errorf("name = %q, want demo-imagestore", dep.Name)
	}
	// dufs serves a single RWO PVC: Recreate avoids two pods on one volume.
	if dep.Spec.Strategy.Type != "Recreate" {
		t.Errorf("strategy = %q, want Recreate", dep.Spec.Strategy.Type)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Errorf("replicas = %v, want 1", dep.Spec.Replicas)
	}

	pod := dep.Spec.Template.Spec
	if pod.SecurityContext == nil || !ptr.Deref(pod.SecurityContext.RunAsNonRoot, false) {
		t.Error("pod runAsNonRoot should be true")
	}
	if dep.Spec.Template.Annotations["checksum/credentials"] != "credhash" {
		t.Error("credentials checksum annotation not set")
	}

	if len(pod.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Containers))
	}
	c := pod.Containers[0]

	if !ptr.Deref(c.SecurityContext.ReadOnlyRootFilesystem, false) {
		t.Error("readOnlyRootFilesystem should be true")
	}
	if c.Image != "sigoden/dufs:latest" {
		t.Errorf("image = %q, want sigoden/dufs:latest", c.Image)
	}
	if len(c.Ports) != 1 || c.Ports[0].ContainerPort != imageStorePort {
		t.Errorf("container port = %v, want %d", c.Ports, imageStorePort)
	}

	// PVC volume mounted at the data path.
	var dataMount *corev1.VolumeMount
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == "data" {
			dataMount = &c.VolumeMounts[i]
		}
	}
	if dataMount == nil || dataMount.MountPath != imageStoreDataPath {
		t.Errorf("data mount = %v, want mountPath %q", dataMount, imageStoreDataPath)
	}
	var dataVol *corev1.Volume
	for i := range pod.Volumes {
		if pod.Volumes[i].Name == "data" {
			dataVol = &pod.Volumes[i]
		}
	}
	if dataVol == nil || dataVol.PersistentVolumeClaim == nil ||
		dataVol.PersistentVolumeClaim.ClaimName != "demo-imagestore" {
		t.Errorf("data volume PVC = %v, want claim demo-imagestore", dataVol)
	}
}

func TestBuildImageStoreDeployment_EnvAndAuth(t *testing.T) {
	gf := imageStoreGoFrame()

	dep := buildImageStoreDeployment(gf, "")
	c := dep.Spec.Template.Spec.Containers[0]

	env := map[string]corev1.EnvVar{}
	for _, e := range c.Env {
		env[e.Name] = e
	}

	wantValues := map[string]string{
		"DUFS_SERVE_PATH":   imageStoreDataPath,
		"DUFS_BIND":         "0.0.0.0",
		"DUFS_PORT":         "5000",
		"DUFS_PATH_PREFIX":  "/gallery",
		"DUFS_ALLOW_UPLOAD": "true",
		"DUFS_ALLOW_DELETE": "true",
	}
	for k, want := range wantValues {
		if got := env[k].Value; got != want {
			t.Errorf("env %s = %q, want %q", k, got, want)
		}
	}

	// The health probe targets the dufs health endpoint under the path prefix.
	if p := c.ReadinessProbe; p == nil || p.HTTPGet == nil || p.HTTPGet.Path != "/gallery/__dufs__/health" {
		t.Errorf("readiness probe path = %v, want /gallery/__dufs__/health", p)
	}

	// Credentials sourced from the Secret, never inlined as plaintext.
	for _, key := range []string{"IMAGESTORE_USERNAME", "IMAGESTORE_PASSWORD"} {
		e, ok := env[key]
		if !ok {
			t.Fatalf("env %s missing", key)
		}
		if e.Value != "" {
			t.Errorf("env %s should have empty Value (sourced from secret), got %q", key, e.Value)
		}
		if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil ||
			e.ValueFrom.SecretKeyRef.Name != "demo-imagestore-credentials" {
			t.Errorf("env %s should reference the credentials secret", key)
		}
	}

	// The --auth arg references the env vars, keeping plaintext out of the pod spec.
	foundAuth := false
	for i, a := range c.Args {
		if a == "--auth" && i+1 < len(c.Args) {
			foundAuth = true
			if c.Args[i+1] != "$(IMAGESTORE_USERNAME):$(IMAGESTORE_PASSWORD)@/:rw|@/" {
				t.Errorf("auth rule = %q", c.Args[i+1])
			}
		}
	}
	if !foundAuth {
		t.Error("expected --auth arg when a secretRef is set")
	}
}

func TestBuildImageStoreDeployment_NoSecretNoAuth(t *testing.T) {
	gf := imageStoreGoFrame()
	gf.Spec.ImageStore.SecretRef = ""

	dep := buildImageStoreDeployment(gf, "")
	c := dep.Spec.Template.Spec.Containers[0]

	for _, a := range c.Args {
		if a == "--auth" {
			t.Error("no --auth arg expected when secretRef is empty")
		}
	}
	for _, e := range c.Env {
		if e.Name == "IMAGESTORE_USERNAME" || e.Name == "IMAGESTORE_PASSWORD" {
			t.Errorf("no credential env expected when secretRef is empty, got %s", e.Name)
		}
	}
}

// TestBuildImageStoreDeployment_CustomPathPrefix guards an overridden subpath:
// a non-default prefix flows into DUFS_PATH_PREFIX and the probe path.
func TestBuildImageStoreDeployment_CustomPathPrefix(t *testing.T) {
	gf := imageStoreGoFrame()
	gf.Spec.ImageStore.PathPrefix = "/images-ui"

	dep := buildImageStoreDeployment(gf, "")
	c := dep.Spec.Template.Spec.Containers[0]

	var prefix string
	found := false
	for _, e := range c.Env {
		if e.Name == "DUFS_PATH_PREFIX" {
			prefix, found = e.Value, true
		}
	}
	if !found || prefix != "/images-ui" {
		t.Errorf("DUFS_PATH_PREFIX = %q (found=%v), want /images-ui", prefix, found)
	}
	if p := c.ReadinessProbe; p == nil || p.HTTPGet == nil || p.HTTPGet.Path != "/images-ui/__dufs__/health" {
		t.Errorf("readiness probe path = %v, want /images-ui/__dufs__/health", p)
	}
}

func TestImageStoreImageRef_Override(t *testing.T) {
	gf := imageStoreGoFrame()
	gf.Spec.ImageStore.Image = goframev1alpha1.ImageSpec{
		Repository: "ghcr.io/example/dufs",
		Tag:        "v1.2.3",
	}
	if got := imageStoreImageRef(gf); got != "ghcr.io/example/dufs:v1.2.3" {
		t.Errorf("imageStoreImageRef = %q", got)
	}
}
