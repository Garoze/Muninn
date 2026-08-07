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

	// A ConfigMap whose keys were all removed arrives from the API server with
	// nil Data. That has to extract to an empty map, not nil: nil means
	// "unreadable", which leaves the removed keys cached.
	t.Run("nil Data returns an empty non-nil map", func(t *testing.T) {
		got := src.Extract(&corev1.ConfigMap{})
		if got == nil {
			t.Fatal("got nil, want an empty non-nil map")
		}
		if len(got) != 0 {
			t.Errorf("got %+v, want empty", got)
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
	got := toAnyMap(nil)
	if got == nil {
		t.Error("nil input: got nil, want an empty non-nil map")
	}
	if len(got) != 0 {
		t.Errorf("nil input: got %+v, want empty", got)
	}

	got = toAnyMap(map[string]string{"a": "1", "b": "2"})
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %+v", got)
	}
}
