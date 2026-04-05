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
	ctrl "sigs.k8s.io/controller-runtime"
	"gopkg.in/yaml.v3"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

const (
	serverPort     int32  = 8080
	serverBinary         = "goframe-server"
	defaultLogLevel      = "info"
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
	if err := r.reconcileServerDeployment(ctx, gf, configData); err != nil {
		return err
	}
	return r.reconcileServerService(ctx, gf)
}

// buildServerConfig renders the server config.yaml content for the given GoFrame spec.
func buildServerConfig(gf *goframev1alpha1.GoFrame) (string, error) {
	type dbConfig struct {
		Type             string `yaml:"type"`
		ConnectionString string `yaml:"connectionString"`
		Namespace        string `yaml:"namespace"`
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
			Type:             "redis",
			ConnectionString: gf.Spec.Redis.Address,
			Namespace:        gf.Name,
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

func (r *GoFrameReconciler) reconcileServerDeployment(ctx context.Context, gf *goframev1alpha1.GoFrame, configData string) error {
	replicas := int32(1)
	img := serverImageRef(gf)
	port := gf.Spec.Server.Port
	if port == 0 {
		port = serverPort
	}

	configHash := fmt.Sprintf("%x", sha256.Sum256([]byte(configData)))

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName(gf),
			Namespace: gf.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: serverLabels(gf),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: serverLabels(gf),
					Annotations: map[string]string{
						"checksum/config": configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            serverBinary,
							Image:           img,
							ImagePullPolicy: imagePullPolicy(gf.Spec.Server.Image.PullPolicy),
							Ports: []corev1.ContainerPort{
								{ContainerPort: port, Protocol: corev1.ProtocolTCP},
							},
							Args: []string{"--config", "/etc/goframe/config.yaml"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/goframe",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: serverName(gf),
									},
								},
							},
						},
					},
				},
			},
		},
	}
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
	if existingContainer.Image != desiredContainer.Image ||
		existingContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy ||
		existingHash != configHash {
		existingContainer.Image = desiredContainer.Image
		existingContainer.ImagePullPolicy = desiredContainer.ImagePullPolicy
		if existing.Spec.Template.Annotations == nil {
			existing.Spec.Template.Annotations = map[string]string{}
		}
		existing.Spec.Template.Annotations["checksum/config"] = configHash
		return r.Update(ctx, existing)
	}
	return nil
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
