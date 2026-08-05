package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/garoze/muninn/internal/config"
)

func TestConfigMapSource_KindWatchListLabelSelectorScope(t *testing.T) {
	src := NewConfigMapSource(&config.Config{ConfigMapLabelSelector: "muninn.io/config=runtime"})

	if src.Kind() != "ConfigMap" {
		t.Errorf("Kind: got %q, want ConfigMap", src.Kind())
	}
	if _, ok := src.Watch().(*corev1.ConfigMap); !ok {
		t.Errorf("Watch: got %T, want *corev1.ConfigMap", src.Watch())
	}
	if _, ok := src.List().(*corev1.ConfigMapList); !ok {
		t.Errorf("List: got %T, want *corev1.ConfigMapList", src.List())
	}
	if src.LabelSelector() != "muninn.io/config=runtime" {
		t.Errorf("LabelSelector: got %q, want muninn.io/config=runtime", src.LabelSelector())
	}
	if src.Scope() != "namespace" {
		t.Errorf("Scope: got %q, want namespace", src.Scope())
	}
}

// TestConfigMapSource_KeyPrefixDefaultsToKind locks in that a single source
// per Kind - every source registered in this repo today - produces the same
// cache key it always has, unaffected by KeyPrefix's addition.
func TestConfigMapSource_KeyPrefixDefaultsToKind(t *testing.T) {
	src := NewConfigMapSource(&config.Config{})
	if src.KeyPrefix() != src.Kind() {
		t.Errorf("KeyPrefix: got %q, want it to default to Kind() %q", src.KeyPrefix(), src.Kind())
	}
}

func TestConfigMapSource_Extract(t *testing.T) {
	src := NewConfigMapSource(&config.Config{})

	t.Run("wrong type returns nil", func(t *testing.T) {
		if got := src.Extract(&corev1.Namespace{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("nil Data returns nil", func(t *testing.T) {
		if got := src.Extract(&corev1.ConfigMap{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("extracts Data as map[string]any", func(t *testing.T) {
		got := src.Extract(&corev1.ConfigMap{Data: map[string]string{"a": "1", "b": "2"}})
		if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
			t.Errorf("got %+v", got)
		}
	})
}

func TestToAnyMap(t *testing.T) {
	if got := toAnyMap(nil); got != nil {
		t.Errorf("nil input: got %+v, want nil", got)
	}

	got := toAnyMap(map[string]string{"a": "1", "b": "2"})
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %+v", got)
	}
}
