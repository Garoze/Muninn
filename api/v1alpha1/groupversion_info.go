package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "muninn.io", Version: "v1alpha1"}
	SchemeBuilder = runtime.NewSchemeBuilder()
	AddToScheme   = SchemeBuilder.AddToScheme
)
