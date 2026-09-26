package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"gopkg.in/yaml.v3"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

const (
	serverPort      int32  = 8080
	serverBinary          = "goframe-server"
	defaultLogLevel       = "info"
)

// serverName returns the name for server resources owned by a GoFrame CR.
func serverName(gf *goframev1alpha1.GoFrame) string {
	return gf.Name + "-server"
}

// reconcileServer ensures the server Deployment, Service, and ConfigMap exist and match the CR spec.
func (r *GoFrameReconciler) reconcileServer(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	configData, err := r.reconcileServerConfigMap(ctx, gf)
	if err != nil {
		return err
	}
	accessKey, secretKey, err := r.readRustFSCredentials(ctx, gf)
	if err != nil {
		return err
	}
	credHash := fmt.Sprintf("%x", sha256.Sum256([]byte(accessKey+":"+secretKey)))
	if err := r.reconcileServerDeployment(ctx, gf, configData, credHash); err != nil {
		return err
	}
	return r.reconcileServerService(ctx, gf)
}

// buildServerConfig renders the server config.yaml content for the given GoFrame spec.
func buildServerConfig(gf *goframev1alpha1.GoFrame) (string, error) {
	type dbConfig struct {
		Type         string `yaml:"type"`
		Endpoint     string `yaml:"endpoint"`
		Bucket       string `yaml:"bucket"`
		AccessKey    string `yaml:"accessKey,omitempty"`
		SecretKey    string `yaml:"secretKey,omitempty"`
		ImageBaseURL string `yaml:"imageBaseURL,omitempty"`
	}
	type cmdConfig struct {
		Name   string         `yaml:"name"`
		Params map[string]any `yaml:",inline"`
	}
	type serverConfig struct {
		Port                          int32       `yaml:"port"`
		LogLevel                      string      `yaml:"logLevel"`
		ThumbnailWidth                int         `yaml:"thumbnailWidth"`
		SvgFallbackLongSidePixelCount int         `yaml:"svgFallbackLongSidePixelCount"`
		Timezone                      string      `yaml:"timezone"`
		Database                      dbConfig    `yaml:"database"`
		Commands                      []cmdConfig `yaml:"commands,omitempty"`
	}

	spec := gf.Spec
	port := spec.Server.Port
	if port == 0 {
		port = serverPort
	}
	logLevel := spec.Server.LogLevel
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	thumbnailWidth := spec.Server.ThumbnailWidth
	if thumbnailWidth == 0 {
		thumbnailWidth = 512
	}
	svgFallback := spec.Server.SvgFallbackLongSidePixelCount
	if svgFallback == 0 {
		svgFallback = 4096
	}
	tz := spec.Timezone
	if tz == "" {
		tz = "UTC"
	}

	bucket := spec.RustFS.Bucket
	if bucket == "" {
		bucket = gf.Name
	}

	cmds := make([]cmdConfig, 0, len(spec.Commands))
	for _, c := range spec.Commands {
		cc := cmdConfig{Name: c.Name}
		if c.Params != nil {
			if err := json.Unmarshal(c.Params.Raw, &cc.Params); err != nil {
				return "", fmt.Errorf("unmarshalling params for command %q: %w", c.Name, err)
			}
		}
		cmds = append(cmds, cc)
	}

	cfg := serverConfig{
		Port:                          port,
		LogLevel:                      logLevel,
		ThumbnailWidth:                thumbnailWidth,
		SvgFallbackLongSidePixelCount: svgFallback,
		Timezone:                      tz,
		Database: dbConfig{
			Type:         "rustfs",
			Endpoint:     spec.RustFS.Endpoint,
			Bucket:       bucket,
			ImageBaseURL: spec.RustFS.ImageBaseURL,
		},
		Commands: cmds,
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (r *GoFrameReconciler) reconcileServerConfigMap(ctx context.Context, gf *goframev1alpha1.GoFrame) (string, error) {
	configData, err := buildServerConfig(gf)
	if err != nil {
		return "", fmt.Errorf("building server config: %w", err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName(gf),
			Namespace: gf.Namespace,
		},
		Data: map[string]string{
			"config.yaml": configData,
		},
	}
	if err := ctrl.SetControllerReference(gf, desired, r.Scheme); err != nil {
		return "", err
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return configData, r.Create(ctx, desired)
	}
	if err != nil {
		return "", err
	}
	if !equality.Semantic.DeepEqual(existing.Data, desired.Data) {
		existing.Data = desired.Data
		return configData, r.Update(ctx, existing)
	}
	return configData, nil
}

// buildServerDeployment renders the desired hardened server Deployment for the
// given GoFrame spec. It is pure (no client calls) so it can be unit-tested.
func buildServerDeployment(gf *goframev1alpha1.GoFrame, configHash, credHash string) *appsv1.Deployment {
	replicas := int32(1)
	if gf.Spec.Server.Replicas != nil {
		replicas = *gf.Spec.Server.Replicas
	}
	port := gf.Spec.Server.Port
	if port == 0 {
		port = serverPort
	}

	container := corev1.Container{
		Name:            serverBinary,
		Image:           serverImageRef(gf),
		ImagePullPolicy: imagePullPolicy(gf.Spec.Server.Image.PullPolicy),
		Ports: []corev1.ContainerPort{
			{ContainerPort: port, Protocol: corev1.ProtocolTCP},
		},
		Args: []string{"--config", "/etc/goframe/config.yaml"},
		Env:  rustfsServerEnvVars(gf),
		VolumeMounts: []corev1.VolumeMount{
			{Name: "config", MountPath: "/etc/goframe", ReadOnly: true},
			{Name: "tmp", MountPath: "/tmp"},
		},
		ReadinessProbe:  serverProbe(port, 5, 10),
		LivenessProbe:   serverProbe(port, 15, 20),
		StartupProbe:    serverStartupProbe(port),
		SecurityContext: serverContainerSecurityContext(),
	}
	if res := gf.Spec.Server.Resources; res != nil {
		container.Resources = *res
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName(gf),
			Namespace: gf.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: serverLabels(gf),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: serverLabels(gf),
					Annotations: map[string]string{
						"checksum/config":      configHash,
						"checksum/credentials": credHash,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: serverPodSecurityContext(),
					Containers:      []corev1.Container{container},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: serverName(gf)},
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
}

func (r *GoFrameReconciler) reconcileServerDeployment(ctx context.Context, gf *goframev1alpha1.GoFrame, configData string, credHash string) error {
	configHash := fmt.Sprintf("%x", sha256.Sum256([]byte(configData)))
	desired := buildServerDeployment(gf, configHash, credHash)
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
	existingHash := existing.Spec.Template.Annotations["checksum/config"]
	existingCredHash := existing.Spec.Template.Annotations["checksum/credentials"]
	needsUpdate := existingContainer.Image != desiredContainer.Image ||
		existingContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy ||
		existingHash != configHash ||
		existingCredHash != credHash ||
		!equalInt32Ptr(existing.Spec.Replicas, desired.Spec.Replicas) ||
		existing.Spec.Strategy.Type != desired.Spec.Strategy.Type ||
		!equality.Semantic.DeepEqual(existingContainer.Env, desiredContainer.Env) ||
		!equality.Semantic.DeepEqual(existing.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers)
	if needsUpdate {
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.Strategy = desired.Spec.Strategy
		existing.Spec.Template.Spec = desired.Spec.Template.Spec
		if existing.Spec.Template.Annotations == nil {
			existing.Spec.Template.Annotations = map[string]string{}
		}
		existing.Spec.Template.Annotations["checksum/config"] = configHash
		existing.Spec.Template.Annotations["checksum/credentials"] = credHash
		return r.Update(ctx, existing)
	}
	return nil
}

// equalInt32Ptr reports whether two *int32 values are equal, treating nil as 0.
func equalInt32Ptr(a, b *int32) bool {
	av, bv := int32(0), int32(0)
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}

// serverProbe builds an HTTP GET readiness/liveness probe against the /probe
// endpoint the server already serves on its container port.
func serverProbe(port, initialDelay, period int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/probe",
				Port: intstr.FromInt32(port),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       period,
	}
}

// serverStartupProbe gives a slow-starting server up to ~30s (10 * 3s) to come
// up before liveness starts killing it.
func serverStartupProbe(port int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/probe",
				Port: intstr.FromInt32(port),
			},
		},
		PeriodSeconds:    3,
		FailureThreshold: 10,
	}
}

// serverPodSecurityContext returns a hardened pod-level security context:
// run as an unprivileged user and default seccomp profile.
func serverPodSecurityContext() *corev1.PodSecurityContext {
	runAsUser := int64(65532) // nonroot user baked into distroless/static images
	return &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsUser,
		FSGroup:      &runAsUser,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// serverContainerSecurityContext returns a hardened container-level security
// context following Kubernetes restricted-profile best practices.
func serverContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		ReadOnlyRootFilesystem:   ptr.To(true),
		RunAsNonRoot:             ptr.To(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

func (r *GoFrameReconciler) reconcileServerService(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	port := gf.Spec.Server.Port
	if port == 0 {
		port = serverPort
	}
	svcType := corev1.ServiceType(gf.Spec.Server.ServiceType)
	if svcType == "" {
		svcType = corev1.ServiceTypeClusterIP
	}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName(gf),
			Namespace: gf.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: serverLabels(gf),
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
	if existing.Spec.Type != desired.Spec.Type {
		existing.Spec.Type = desired.Spec.Type
		existing.Spec.Ports = desired.Spec.Ports
		return r.Update(ctx, existing)
	}
	return nil
}

func serverLabels(gf *goframev1alpha1.GoFrame) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "goframe",
		"app.kubernetes.io/instance":  gf.Name,
		"app.kubernetes.io/component": "server",
	}
}

func serverImageRef(gf *goframev1alpha1.GoFrame) string {
	img := gf.Spec.Server.Image
	repo := img.Repository
	if repo == "" {
		repo = "ghcr.io/jo-hoe/goframe"
	}
	tag := img.Tag
	if tag == "" {
		tag = "latest"
	}
	return repo + ":" + tag
}

func imagePullPolicy(policy string) corev1.PullPolicy {
	switch policy {
	case "Always":
		return corev1.PullAlways
	case "Never":
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}

// rustfsServerEnvVars returns RUSTFS_ACCESS_KEY/RUSTFS_SECRET_KEY env vars
// for the goframe server container, read from the RustFS credentials Secret.
func rustfsServerEnvVars(gf *goframev1alpha1.GoFrame) []corev1.EnvVar {
	ref := gf.Spec.RustFS.SecretRef
	if ref == "" {
		return nil
	}
	return []corev1.EnvVar{
		{
			Name: "RUSTFS_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref},
					Key:                  "accessKey",
				},
			},
		},
		{
			Name: "RUSTFS_SECRET_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref},
					Key:                  "secretKey",
				},
			},
		},
	}
}
