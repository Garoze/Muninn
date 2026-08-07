package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func TestNewScheme_RegistersConfigMap(t *testing.T) {
	s, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	if !s.Recognizes(corev1.SchemeGroupVersion.WithKind("ConfigMap")) {
		t.Error("scheme does not recognize corev1.ConfigMap - Watcher can't watch it")
	}
}

func TestNewScheme_RegistersSecretProviderClass(t *testing.T) {
	s, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	if !s.Recognizes(secretsstorev1.SchemeGroupVersion.WithKind("SecretProviderClass")) {
		t.Error("scheme does not recognize SecretProviderClass - ReconcileSecretProviderClass can't Get/Patch it")
	}
	if !s.Recognizes(secretsstorev1.SchemeGroupVersion.WithKind("SecretProviderClassPodStatus")) {
		t.Error("scheme does not recognize SecretProviderClassPodStatus")
	}
}

func TestNewScheme_DoesNotRecognizeArbitraryUnregisteredKind(t *testing.T) {
	// Negative path: a scheme that recognizes everything by accident would
	// hide real registration bugs (e.g. a typo'd group) behind an
	// always-true Recognizes call.
	s, err := NewScheme()
	if err != nil {
		t.Fatalf("NewScheme: %v", err)
	}
	if s.Recognizes(secretsstorev1.SchemeGroupVersion.WithKind("NotARealKind")) {
		t.Error("scheme unexpectedly recognizes a made-up Kind")
	}
}
