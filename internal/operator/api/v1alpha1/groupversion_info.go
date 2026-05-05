// Package v1alpha1 defines the GoFrame CRD API types.
// +groupName=goframe.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the API group and version for GoFrame resources.
	GroupVersion = schema.GroupVersion{Group: "goframe.io", Version: "v1alpha1"}

	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &GoFrame{}, &GoFrameList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
