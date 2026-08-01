package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "muninn.io", Version: "v1alpha1"}

	SchemeBuilder = &runtime.SchemeBuilder{}

	AddToScheme = SchemeBuilder.AddToScheme
)

// addKnownTypes registers the given objects under GroupVersion and the
// common list/watch metadata types for that group version.
func addKnownTypes(objects ...runtime.Object) {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, objects...)
		metav1.AddToGroupVersion(s, GroupVersion)
		return nil
	})
}
