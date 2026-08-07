package kube

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

// NewScheme creates a runtime.Scheme that knows about core Kubernetes types,
// including corev1.ConfigMap plus the SecretProviderClass CRD.
func NewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, err
	}

	if err := secretsstorev1.Install(s); err != nil {
		return nil, err
	}

	return s, nil
}
