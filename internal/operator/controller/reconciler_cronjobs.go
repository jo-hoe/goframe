package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"gopkg.in/yaml.v3"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

const (
	schedulerInstanceLabel  = "goframe-instance"
	schedulerNameLabel      = "goframe-scheduler"
	schedulerConfigMountPath = "/etc/goframe-scheduler"
	schedulerConfigFileName  = "image-scheduler.yaml"
)

// reconcileCronJobs diffs spec.schedulers against existing CronJobs and
// creates, updates, or deletes them to match the desired state.
func (r *GoFrameReconciler) reconcileCronJobs(ctx context.Context, gf *goframev1alpha1.GoFrame) error {
	// List all CronJobs owned by this GoFrame CR.
	existing := &batchv1.CronJobList{}
	if err := r.List(ctx, existing,
		client.InNamespace(gf.Namespace),
		client.MatchingLabels{schedulerInstanceLabel: gf.Name},
	); err != nil {
		return fmt.Errorf("listing cronjobs: %w", err)
	}

	// Build a map of existing CronJobs by scheduler name.
	existingByName := make(map[string]*batchv1.CronJob, len(existing.Items))
	for i := range existing.Items {
		name := existing.Items[i].Labels[schedulerNameLabel]
		existingByName[name] = &existing.Items[i]
	}

	// Build desired set.
	desiredNames := make(map[string]struct{}, len(gf.Spec.Schedulers))
	for _, sched := range gf.Spec.Schedulers {
		desiredNames[sched.Name] = struct{}{}

		if err := r.reconcileSchedulerConfigMap(ctx, gf, sched); err != nil {
			return fmt.Errorf("reconciling configmap for scheduler %q: %w", sched.Name, err)
		}

		desired := r.buildCronJob(gf, sched)
		if err := ctrl.SetControllerReference(gf, desired, r.Scheme); err != nil {
			return err
		}

		curr, exists := existingByName[sched.Name]
		if !exists {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("creating cronjob %q: %w", sched.Name, err)
			}
			continue
		}

		// Update if schedule, timezone, image, or volume config changed.
		currContainer := curr.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
		desiredContainer := desired.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
		currTZ := ""
		if curr.Spec.TimeZone != nil {
			currTZ = *curr.Spec.TimeZone
		}
		if curr.Spec.Schedule != desired.Spec.Schedule ||
			currTZ != *desired.Spec.TimeZone ||
			currContainer.Image != desiredContainer.Image ||
			!equality.Semantic.DeepEqual(currContainer.Env, desiredContainer.Env) ||
			!equality.Semantic.DeepEqual(currContainer.VolumeMounts, desiredContainer.VolumeMounts) ||
			!equality.Semantic.DeepEqual(curr.Spec.JobTemplate.Spec.Template.Spec.Volumes, desired.Spec.JobTemplate.Spec.Template.Spec.Volumes) {
			curr.Spec.Schedule = desired.Spec.Schedule
			curr.Spec.JobTemplate = desired.Spec.JobTemplate
			if err := r.Update(ctx, curr); err != nil {
				return fmt.Errorf("updating cronjob %q: %w", sched.Name, err)
			}
		}
	}

	// Delete CronJobs that are no longer in spec.
	for name, cj := range existingByName {
		if _, wanted := desiredNames[name]; !wanted {
			if err := r.Delete(ctx, cj); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting cronjob %q: %w", name, err)
			}
		}
	}

	return nil
}

// buildSchedulerConfig renders the scheduler image-scheduler.yaml content for the given SchedulerSpec.
func buildSchedulerConfig(gf *goframev1alpha1.GoFrame, sched goframev1alpha1.SchedulerSpec) (string, error) {
	type sourceConfig struct {
		Enabled bool `yaml:"enabled"`
	}
	type sourcesConfig struct {
		XKCD sourceConfig `yaml:"xkcd"`
	}
	type cmdConfig struct {
		Name   string         `yaml:"name"`
		Params map[string]any `yaml:",inline"`
	}
	type schedulerConfig struct {
		GoframeURL                  string        `yaml:"goframeURL"`
		SourceName                  string        `yaml:"sourceName"`
		KeepCount                   int           `yaml:"keepCount"`
		DrainIfUnmanagedImagesExceed int           `yaml:"drainIfUnmanagedImagesExceed"`
		LogLevel                    string        `yaml:"logLevel"`
		Sources                     sourcesConfig `yaml:"sources"`
		Commands                    []cmdConfig   `yaml:"commands,omitempty"`
	}

	keepCount := sched.KeepCount
	if keepCount < 1 {
		keepCount = 1
	}
	logLevel := sched.LogLevel
	if logLevel == "" {
		logLevel = defaultLogLevel
	}

	var sources sourcesConfig
	switch strings.ToLower(sched.Source) {
	case "xkcd":
		sources.XKCD.Enabled = true
	}

	cmds := make([]cmdConfig, 0, len(sched.Commands))
	for _, c := range sched.Commands {
		cc := cmdConfig{Name: c.Name}
		if c.Params != nil {
			if err := json.Unmarshal(c.Params.Raw, &cc.Params); err != nil {
				return "", fmt.Errorf("unmarshalling params for command %q: %w", c.Name, err)
			}
		}
		cmds = append(cmds, cc)
	}

	cfg := schedulerConfig{
		GoframeURL:                  serverURL(gf),
		SourceName:                  sched.Source,
		KeepCount:                   keepCount,
		DrainIfUnmanagedImagesExceed: sched.DrainIfUnmanagedImagesExceed,
		LogLevel:                    logLevel,
		Sources:                     sources,
		Commands:                    cmds,
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (r *GoFrameReconciler) reconcileSchedulerConfigMap(ctx context.Context, gf *goframev1alpha1.GoFrame, sched goframev1alpha1.SchedulerSpec) error {
	configData, err := buildSchedulerConfig(gf, sched)
	if err != nil {
		return fmt.Errorf("building scheduler config: %w", err)
	}

	cmName := fmt.Sprintf("%s-sched-%s", gf.Name, sched.Name)
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: gf.Namespace,
		},
		Data: map[string]string{
			schedulerConfigFileName: configData,
		},
	}
	if err := ctrl.SetControllerReference(gf, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.Data, desired.Data) {
		existing.Data = desired.Data
		return r.Update(ctx, existing)
	}
	return nil
}

// buildCronJob constructs the desired CronJob for a SchedulerSpec.
func (r *GoFrameReconciler) buildCronJob(gf *goframev1alpha1.GoFrame, sched goframev1alpha1.SchedulerSpec) *batchv1.CronJob {
	img := schedulerImageRef(sched)

	name := fmt.Sprintf("%s-sched-%s", gf.Name, sched.Name)
	labels := map[string]string{
		schedulerInstanceLabel: gf.Name,
		schedulerNameLabel:     sched.Name,
	}

	configMountPath := schedulerConfigMountPath + "/" + schedulerConfigFileName

	tz := gf.Spec.Timezone
	if tz == "" {
		tz = "UTC"
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: gf.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   sched.Cron,
			TimeZone:                   &tz,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: int32Ptr(1),
			FailedJobsHistoryLimit:     int32Ptr(3),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:            "scheduler",
									Image:           img,
									ImagePullPolicy: imagePullPolicy(sched.Image.PullPolicy),
									Env: []corev1.EnvVar{
										{
											Name:  "IMAGE_SCHEDULER_CONFIG_PATH",
											Value: configMountPath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config",
											MountPath: schedulerConfigMountPath,
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
												Name: name,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func schedulerImageRef(sched goframev1alpha1.SchedulerSpec) string {
	repo := sched.Image.Repository
	if repo == "" {
		repo = "ghcr.io/jo-hoe/goframe-image-scheduler"
	}
	tag := sched.Image.Tag
	if tag == "" {
		tag = "latest"
	}
	return repo + ":" + tag
}

func serverURL(gf *goframev1alpha1.GoFrame) string {
	port := gf.Spec.Server.Port
	if port == 0 {
		port = serverPort
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serverName(gf), gf.Namespace, port)
}

func int32Ptr(v int32) *int32 {
	return &v
}
