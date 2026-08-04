package kube

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/garoze/muninn/internal/config"
)

// ConfigSource is a pluggable, informer-backed source of configuration
// data. Each source owns its own informer and contributes its own named
// slice to the per-namespace patch-merge in app.ConfigEntry.
type ConfigSource interface {
	// Kind identifies this source for logging, Describe, and as a
	// namespacing prefix on the ConfigEntry.Sources key (so a ConfigMap
	// and a same-named custom resource in the same namespace don't
	// collide as merge sources).
	Kind() string

	// Watch returns an empty instance of the object type to watch.
	Watch() client.Object

	// List returns an empty instance of the corresponding list type, used
	// to seed the cache from the informer's already-synced store.
	List() client.ObjectList

	// LabelSelector scopes which objects of that type are watched.
	LabelSelector() string

	// Scope describes what the watched object is scoped by (e.g.
	// "namespace"), reported via Describe.
	Scope() string

	// Extract pulls configuration data out of a matching object.
	Extract(obj client.Object) map[string]any
}

// ConfigMapSource watches corev1.ConfigMap, scoped by a label selector, and
// extracts its Data as the config slice. The default/reference ConfigSource
// implementation.
type ConfigMapSource struct {
	labelSelector string
}

// NewConfigMapSource creates a ConfigMapSource scoped to
// cfg.ConfigMapLabelSelector.
func NewConfigMapSource(cfg *config.Config) *ConfigMapSource {
	return &ConfigMapSource{labelSelector: cfg.ConfigMapLabelSelector}
}

func (s *ConfigMapSource) Kind() string { return "ConfigMap" }

func (s *ConfigMapSource) Watch() client.Object { return &corev1.ConfigMap{} }

func (s *ConfigMapSource) List() client.ObjectList { return &corev1.ConfigMapList{} }

func (s *ConfigMapSource) LabelSelector() string { return s.labelSelector }

func (s *ConfigMapSource) Scope() string { return "namespace" }

func (s *ConfigMapSource) Extract(obj client.Object) map[string]any {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok || cm == nil {
		return nil
	}
	return toAnyMap(cm.Data)
}

func toAnyMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}

// SecretSource watches corev1.Secret, scoped by a label selector, and
// extracts its Data as the config slice. A second, bring-your-own-style
// ConfigSource implementation alongside ConfigMapSource, proving the
// interface generalizes beyond the reference type: a different Kubernetes
// kind, a different label selector, and (unlike ConfigMapSource) a decode
// step, since corev1.Secret.Data is raw bytes rather than strings.
//
// A Secret and a ConfigMap can legitimately share the same name in the same
// namespace (e.g. both named "app-config") without colliding in the merged
// cache, because sourceKey prefixes each contributed slice with Kind() -
// "ConfigMap/app-config" and "Secret/app-config" are distinct
// ConfigEntry.Sources keys even though the underlying object names match.
type SecretSource struct {
	labelSelector string
}

// NewSecretSource creates a SecretSource scoped to cfg.SecretLabelSelector.
func NewSecretSource(cfg *config.Config) *SecretSource {
	return &SecretSource{labelSelector: cfg.SecretLabelSelector}
}

func (s *SecretSource) Kind() string { return "Secret" }

func (s *SecretSource) Watch() client.Object { return &corev1.Secret{} }

func (s *SecretSource) List() client.ObjectList { return &corev1.SecretList{} }

func (s *SecretSource) LabelSelector() string { return s.labelSelector }

func (s *SecretSource) Scope() string { return "namespace" }

func (s *SecretSource) Extract(obj client.Object) map[string]any {
	sec, ok := obj.(*corev1.Secret)
	if !ok || sec == nil || sec.Data == nil {
		return nil
	}

	out := make(map[string]any, len(sec.Data))
	for k, v := range sec.Data {
		out[k] = string(v)
	}

	return out
}
