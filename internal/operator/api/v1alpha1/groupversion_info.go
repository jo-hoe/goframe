// Package v1alpha1 defines the GoFrame CRD API types.
// +groupName=goframe.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is the API group and version for GoFrame resources.
	GroupVersion = schema.GroupVersion{Group: "goframe.io", Version: "v1alpha1"}

	// SchemeBuilder registers GoFrame types with a runtime.Scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&GoFrame{}, &GoFrameList{})
}
