// Package v1alpha1 defines the GoFrame CRD API types.
package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GoFrame is the Schema for the goframes API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CurrentImage",type="string",JSONPath=".status.currentImageID"
// +kubebuilder:printcolumn:name="ServerReady",type="boolean",JSONPath=".status.serverReady"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type GoFrame struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GoFrameSpec   `json:"spec,omitempty"`
	Status GoFrameStatus `json:"status,omitempty"`
}

// GoFrameList contains a list of GoFrame.
// +kubebuilder:object:root=true
type GoFrameList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GoFrame `json:"items"`
}

// GoFrameSpec defines the desired state of GoFrame.
// +kubebuilder:object:generate=true
type GoFrameSpec struct {
	// Timezone for image rotation midnight detection.
	// +kubebuilder:default="UTC"
	// +optional
	Timezone string `json:"timezone,omitempty"`

	// Commands is the image-processing pipeline applied to every ingested image.
	// +optional
	Commands []CommandSpec `json:"commands,omitempty"`

	// Schedulers defines one CronJob per entry for automated image ingestion.
	// +optional
	Schedulers []SchedulerSpec `json:"schedulers,omitempty"`

	// Server configures the goframe server Deployment.
	// +optional
	Server ServerSpec `json:"server,omitempty"`

	// Redis configures the Redis Deployment managed by the operator.
	// +optional
	Redis RedisSpec `json:"redis,omitempty"`
}

// CommandSpec describes a single image-processing command in the pipeline.
// +kubebuilder:object:generate=true
type CommandSpec struct {
	// Name is the registered command name (e.g. "scale", "dither").
	Name string `json:"name"`

	// Params are command-specific parameters. The schema is open to allow diverse
	// command configurations; validated at runtime by the command implementation.
	// +optional
	Params *apiextensionsv1.JSON `json:"params,omitempty"`
}

// SchedulerSpec defines a single CronJob-based image ingestion source.
// +kubebuilder:object:generate=true
type SchedulerSpec struct {
	// Name uniquely identifies this scheduler within the GoFrame instance.
	Name string `json:"name"`

	// Cron is the schedule in standard cron syntax (e.g. "0 8 * * *").
	Cron string `json:"cron"`

	// KeepCount is the maximum number of images to retain from this source.
	// Older images beyond this count are deleted after each run.
	// +kubebuilder:default=1
	// +optional
	KeepCount int `json:"keepCount,omitempty"`

	// WhenUnmanaged controls scheduler behaviour when unmanaged images exist.
	// Valid values: upload (default), skip, drain.
	// +kubebuilder:validation:Enum=upload;skip;drain
	// +optional
	WhenUnmanaged string `json:"whenUnmanaged,omitempty"`

	// ExclusionGroup is an optional group name shared by schedulers that are mutually exclusive.
	// When a scheduler in a group successfully uploads an image, all images owned by other
	// members of the same group are deleted. This enables day-of-week or time-period routing
	// where only one scheduler's images should be present at a time.
	// +optional
	ExclusionGroup string `json:"exclusionGroup,omitempty"`

	// LogLevel sets the scheduler log verbosity (debug, info, warn, error).
	// +kubebuilder:default="info"
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// Commands is the image-processing pipeline applied to images fetched by this scheduler.
	// +optional
	Commands []CommandSpec `json:"commands,omitempty"`

	// Source is the image source identifier (e.g. "xkcd", "oatmeal", "deviantart", "metmuseum", "tumblr", "s3").
	Source string `json:"source"`

	// DeviantArt holds configuration for the deviantart source.
	// Required when source is "deviantart".
	// +optional
	DeviantArt *DeviantArtConfig `json:"deviantart,omitempty"`

	// MetMuseum holds configuration for the metmuseum source.
	// +optional
	MetMuseum *MetMuseumConfig `json:"metmuseum,omitempty"`

	// Tumblr holds configuration for the tumblr source.
	// Required when source is "tumblr".
	// +optional
	Tumblr *TumblrConfig `json:"tumblr,omitempty"`

	// S3 holds configuration for the s3 source (AWS S3, RustFS, MinIO, etc.).
	// Required when source is "s3".
	// +optional
	S3 *S3Config `json:"s3,omitempty"`

	// Image configures the container image for the scheduler CronJob.
	// +optional
	Image ImageSpec `json:"image,omitempty"`
}

// DeviantArtConfig holds the configuration for the deviantart image source.
// +kubebuilder:object:generate=true
type DeviantArtConfig struct {
	// Query is the DeviantArt search string, e.g. "boost:popular traditionalart".
	// Uses DeviantArt RSS query syntax: https://www.deviantart.com/developers/rss
	Query string `json:"query"`
}

// MetMuseumConfig holds the configuration for the metmuseum image source.
// +kubebuilder:object:generate=true
type MetMuseumConfig struct {
	// DepartmentIDs restricts searches to specific Met department IDs.
	// Omit to search all departments.
	// See https://collectionapi.metmuseum.org/public/collection/v1/departments for valid IDs.
	// +optional
	DepartmentIDs []int `json:"departmentIDs,omitempty"`
}

// TumblrConfig holds the configuration for the tumblr image source.
// +kubebuilder:object:generate=true
type TumblrConfig struct {
	// Blogs is the list of Tumblr blog names (e.g. ["nasa", "pusheen"]), without the .tumblr.com suffix.
	// One blog is picked randomly per run.
	Blogs []string `json:"blogs"`
}

// S3Config holds the configuration for the s3 image source.
// Compatible with AWS S3, RustFS, MinIO, and any S3-compatible storage.
// +kubebuilder:object:generate=true
type S3Config struct {
	// Endpoint is the base URL of the S3-compatible service (no trailing slash, no bucket path).
	// For AWS S3: "https://s3.<region>.amazonaws.com".
	// For RustFS / MinIO: "http://rustfs:9000".
	Endpoint string `json:"endpoint"`

	// Bucket is the name of the bucket to fetch images from.
	Bucket string `json:"bucket"`

	// Prefix is an optional key prefix to filter objects (e.g. "photos/").
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// Region is the AWS region identifier (e.g. "us-east-1").
	// For RustFS, any non-empty value is accepted.
	Region string `json:"region"`

	// AccessKey is the access key ID.
	// Leave empty for anonymous access to public buckets.
	// +optional
	AccessKey string `json:"accessKey,omitempty"`

	// SecretKey is the secret access key.
	// Leave empty for anonymous access to public buckets.
	// +optional
	SecretKey string `json:"secretKey,omitempty"`
}

// ServerSpec configures the goframe server Deployment.
// +kubebuilder:object:generate=true
type ServerSpec struct {
	// Image configures the container image for the server Deployment.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Port is the HTTP port the server listens on.
	// +kubebuilder:default=8080
	// +optional
	Port int32 `json:"port,omitempty"`

	// ThumbnailWidth is the pixel width for generated thumbnails.
	// +kubebuilder:default=512
	// +optional
	ThumbnailWidth int `json:"thumbnailWidth,omitempty"`

	// LogLevel sets the server log verbosity (debug, info, warn, error).
	// +kubebuilder:default="info"
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// SvgFallbackLongSidePixelCount is the rasterization size for SVG images.
	// +kubebuilder:default=4096
	// +optional
	SvgFallbackLongSidePixelCount int `json:"svgFallbackLongSidePixelCount,omitempty"`

	// ServiceType is the Kubernetes Service type for the server (ClusterIP, NodePort, LoadBalancer).
	// +kubebuilder:default="ClusterIP"
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +optional
	ServiceType string `json:"serviceType,omitempty"`
}

// RedisSpec configures the Redis connection used by the operator.
// +kubebuilder:object:generate=true
type RedisSpec struct {
	// Address is the Redis connection string (host:port) the operator and server use.
	// Typically points at a Redis instance deployed via a separate Helm chart.
	// +kubebuilder:example="redis-master.default.svc.cluster.local:6379"
	Address string `json:"address"`
}

// ImageSpec selects a container image.
// +kubebuilder:object:generate=true
type ImageSpec struct {
	// Repository is the image repository (e.g. "ghcr.io/jo-hoe/goframe").
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the image tag (e.g. "latest", "v1.2.3").
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy is the Kubernetes image pull policy.
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	// +optional
	PullPolicy string `json:"pullPolicy,omitempty"`
}

// GoFrameStatus defines the observed state of GoFrame.
// +kubebuilder:object:generate=true
type GoFrameStatus struct {
	// CurrentImageID is the ID of the image currently shown by the server.
	// +optional
	CurrentImageID string `json:"currentImageID,omitempty"`

	// LastRotationTime is when the operator last advanced the image rotation.
	// +optional
	LastRotationTime *metav1.Time `json:"lastRotationTime,omitempty"`

	// ServerReady is true when the server Deployment has at least one ready replica.
	ServerReady bool `json:"serverReady"`

	// SchedulerStatuses maps scheduler name to its last-run status.
	// +optional
	SchedulerStatuses map[string]SchedulerStatus `json:"schedulerStatuses,omitempty"`

	// Conditions reports reconciliation state for standard Kubernetes tooling.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// SchedulerStatus summarises the last execution of a named scheduler CronJob.
// +kubebuilder:object:generate=true
type SchedulerStatus struct {
	// LastRunTime is when the CronJob last completed successfully.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// ImagesAdded is the count of images added during the last run.
	ImagesAdded int `json:"imagesAdded"`
}
