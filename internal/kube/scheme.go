package kube

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	v1alpha1 "github.com/garoze/muninn/api/v1alpha1"
)

// NewScheme creates a runtime.Scheme that knows about core Kubernetes types
// and the muninn.io v1alpha1 CRDs (Tenant, TenantConfig, Policy).
func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := v1alpha1.AddToScheme(s); err != nil {
		return nil, err
	}
	return s, nil
}
