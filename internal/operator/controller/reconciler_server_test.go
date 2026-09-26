package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

func testGoFrame() *goframev1alpha1.GoFrame {
	return &goframev1alpha1.GoFrame{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: goframev1alpha1.GoFrameSpec{
			RustFS: goframev1alpha1.RustFSSpec{Endpoint: "http://rustfs:9000"},
		},
	}
}

func TestBuildServerDeployment_Hardening(t *testing.T) {
	gf := testGoFrame()

	dep := buildServerDeployment(gf, "cfghash", "credhash")

	// Rollout strategy is explicit.
	if dep.Spec.Strategy.Type != "RollingUpdate" {
		t.Errorf("strategy = %q, want RollingUpdate", dep.Spec.Strategy.Type)
	}

	// Default replicas.
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Errorf("replicas = %v, want 1", dep.Spec.Replicas)
	}

	pod := dep.Spec.Template.Spec

	// Pod security context.
	if pod.SecurityContext == nil {
		t.Fatal("pod securityContext is nil")
	}
	if !ptr.Deref(pod.SecurityContext.RunAsNonRoot, false) {
		t.Error("pod runAsNonRoot should be true")
	}
	if pod.SecurityContext.SeccompProfile == nil ||
		pod.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("pod seccompProfile should be RuntimeDefault")
	}

	if len(pod.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Containers))
	}
	c := pod.Containers[0]

	// Container security context.
	if c.SecurityContext == nil {
		t.Fatal("container securityContext is nil")
	}
	if ptr.Deref(c.SecurityContext.AllowPrivilegeEscalation, true) {
		t.Error("allowPrivilegeEscalation should be false")
	}
	if !ptr.Deref(c.SecurityContext.ReadOnlyRootFilesystem, false) {
		t.Error("readOnlyRootFilesystem should be true")
	}
	if c.SecurityContext.Capabilities == nil ||
		len(c.SecurityContext.Capabilities.Drop) != 1 ||
		c.SecurityContext.Capabilities.Drop[0] != "ALL" {
		t.Error("container should drop ALL capabilities")
	}

	// Probes target /probe on the container port.
	for name, p := range map[string]*corev1.Probe{
		"readiness": c.ReadinessProbe,
		"liveness":  c.LivenessProbe,
		"startup":   c.StartupProbe,
	} {
		if p == nil || p.HTTPGet == nil {
			t.Fatalf("%s probe missing HTTPGet", name)
		}
		if p.HTTPGet.Path != "/probe" {
			t.Errorf("%s probe path = %q, want /probe", name, p.HTTPGet.Path)
		}
		if p.HTTPGet.Port.IntVal != serverPort {
			t.Errorf("%s probe port = %d, want %d", name, p.HTTPGet.Port.IntVal, serverPort)
		}
	}

	// readOnlyRootFilesystem requires a writable /tmp emptyDir.
	var hasTmpVol bool
	for _, v := range pod.Volumes {
		if v.Name == "tmp" && v.EmptyDir != nil {
			hasTmpVol = true
		}
	}
	if !hasTmpVol {
		t.Error("expected a writable tmp emptyDir volume")
	}
	var hasTmpMount bool
	for _, m := range c.VolumeMounts {
		if m.Name == "tmp" && m.MountPath == "/tmp" {
			hasTmpMount = true
		}
	}
	if !hasTmpMount {
		t.Error("expected /tmp volume mount on the container")
	}
}

func TestBuildServerDeployment_ReplicasAndResources(t *testing.T) {
	gf := testGoFrame()
	gf.Spec.Server.Replicas = ptr.To(int32(3))
	gf.Spec.Server.Resources = &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	dep := buildServerDeployment(gf, "cfg", "cred")

	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 3 {
		t.Errorf("replicas = %v, want 3", dep.Spec.Replicas)
	}
	got := dep.Spec.Template.Spec.Containers[0].Resources
	if got.Requests.Cpu().String() != "50m" {
		t.Errorf("cpu request = %s, want 50m", got.Requests.Cpu())
	}
	if got.Limits.Memory().String() != "512Mi" {
		t.Errorf("memory limit = %s, want 512Mi", got.Limits.Memory())
	}
}

func TestEqualInt32Ptr(t *testing.T) {
	cases := []struct {
		a, b *int32
		want bool
	}{
		{nil, nil, true},
		{ptr.To(int32(0)), nil, true},
		{ptr.To(int32(1)), ptr.To(int32(1)), true},
		{ptr.To(int32(1)), ptr.To(int32(2)), false},
		{ptr.To(int32(1)), nil, false},
	}
	for _, tc := range cases {
		if got := equalInt32Ptr(tc.a, tc.b); got != tc.want {
			t.Errorf("equalInt32Ptr(%v,%v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
