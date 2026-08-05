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
	// Kind identifies this source's type for logging, Describe, and
	// ENABLED_CONFIG_SOURCES matching. Not necessarily unique across
	// registered sources - two sources of the same kind (e.g. ConfigMaps
	// scoped by different label selectors) share a Kind by design.
	Kind() string

	// KeyPrefix namespaces this source's contributed slice within a merge
	// target's Sources map, so co-registered sources don't collide when
	// they share a Kind and watch same-named objects. Distinct from Kind:
	// a source with no reason to differentiate itself from siblings of
	// the same kind can return its own Kind() here.
	KeyPrefix() string

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
	keyPrefix     string
}

// NewConfigMapSource creates a ConfigMapSource scoped to
// cfg.ConfigMapLabelSelector.
func NewConfigMapSource(cfg *config.Config) *ConfigMapSource {
	return &ConfigMapSource{labelSelector: cfg.ConfigMapLabelSelector}
}

func (s *ConfigMapSource) Kind() string { return "ConfigMap" }

// KeyPrefix falls back to Kind when unset, which is every source
// registered today - no caller needs a distinct prefix until a second
// ConfigMapSource instance is registered alongside this one.
func (s *ConfigMapSource) KeyPrefix() string {
	if s.keyPrefix != "" {
		return s.keyPrefix
	}
	return s.Kind()
}

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
