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
	// Kind identifies this source's type, for logging, Describe, and
	// ENABLED_CONFIG_SOURCES matching. Sources of the same type share a
	// Kind by design.
	Kind() string

	// KeyPrefix namespaces this source's slice of a merge target's
	// Sources map, distinct from Kind so two sources sharing a Kind can
	// still avoid colliding on a shared object name.
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

	// Extract pulls configuration data out of a matching object. A non-nil
	// empty map means the object carries no keys, which is distinct from nil:
	// nil means this source cannot read the object at all, and the merge
	// leaves its existing slice untouched rather than emptying it.
	Extract(obj client.Object) map[string]any
}

// ConfigMapSource watches corev1.ConfigMap, scoped by a label selector, and
// extracts its Data as the config slice. The default/reference ConfigSource
// implementation.
type ConfigMapSource struct {
	labelSelector string
	keyPrefix     string
}

// ConfigMapSourceOption customizes a ConfigMapSource at registration.
type ConfigMapSourceOption func(*ConfigMapSource)

// WithKeyPrefix sets this registration's cache-facing identity. Only needed
// when more than one ConfigMapSource is registered: Kind() is identical across
// them, so without distinct prefixes their contributions would be
// indistinguishable in the merge and NewWatcher rejects the registration.
func WithKeyPrefix(prefix string) ConfigMapSourceOption {
	return func(s *ConfigMapSource) { s.keyPrefix = prefix }
}

// WithLabelSelector overrides the selector taken from Config, so a second
// registration can scope itself to a different set of ConfigMaps than the
// default one.
func WithLabelSelector(selector string) ConfigMapSourceOption {
	return func(s *ConfigMapSource) { s.labelSelector = selector }
}

// NewConfigMapSource creates a ConfigMapSource scoped to
// cfg.ConfigMapLabelSelector unless an option overrides it.
func NewConfigMapSource(cfg *config.Config, opts ...ConfigMapSourceOption) *ConfigMapSource {
	s := &ConfigMapSource{labelSelector: cfg.ConfigMapLabelSelector}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *ConfigMapSource) Kind() string { return "ConfigMap" }

// KeyPrefix falls back to Kind when unset - correct as long as only one
// source of this kind is registered.
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

// toAnyMap always returns a non-nil map. The API server stores a ConfigMap
// whose every key was removed as nil Data, and returning nil for that would
// read as "unreadable object" and leave the removed keys cached indefinitely.
func toAnyMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}
