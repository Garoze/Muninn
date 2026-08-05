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
	// namespacing prefix so same-named objects of different kinds don't
	// collide as merge sources.
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
