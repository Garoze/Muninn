package kube

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

func newTestWatcher(t *testing.T) *Watcher {
	t.Helper()
	return &Watcher{
		appCache: app.NewCache(),
		metrics:  observability.NewMetrics(prometheus.NewRegistry()),
		log:      zap.NewNop(),
	}
}

// fakeSource is a minimal ConfigSource used to prove the abstraction
// composes with more than just ConfigMapSource.
type fakeSource struct {
	kind          string
	keyPrefix     string
	labelSelector string
	data          map[string]any
}

func (f *fakeSource) Kind() string { return f.kind }
func (f *fakeSource) KeyPrefix() string {
	if f.keyPrefix != "" {
		return f.keyPrefix
	}
	return f.kind
}
func (f *fakeSource) Watch() client.Object                   { return &corev1.Secret{} }
func (f *fakeSource) List() client.ObjectList                { return &corev1.SecretList{} }
func (f *fakeSource) LabelSelector() string                  { return f.labelSelector }
func (f *fakeSource) Scope() string                          { return "namespace" }
func (f *fakeSource) Extract(_ client.Object) map[string]any { return f.data }

// --- applyPatch: the patch-based cache merge ---

func TestApplyPatch_EmptyNamespaceIsNoop(t *testing.T) {
	w := newTestWatcher(t)
	w.applyPatch(configPatch{namespace: ""})

	if w.appCache.Len() != 0 {
		t.Errorf("got %d cached namespaces, want 0", w.appCache.Len())
	}
}

func TestApplyPatch_CreatesNewEntry(t *testing.T) {
	w := newTestWatcher(t)
	w.applyPatch(configPatch{
		namespace: "ns1",
		source:    "cm1",
		data:      map[string]any{"LOG_LEVEL": "info"},
		revision:  "1",
	})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to be cached")
	}
	if entry.Sources["cm1"]["LOG_LEVEL"] != "info" {
		t.Errorf("got %+v", entry.Sources)
	}
}

// TestApplyPatch_SourceScopedMerge asserts one source's update never
// clobbers another source's data in the same namespace.
func TestApplyPatch_SourceScopedMerge(t *testing.T) {
	w := newTestWatcher(t)

	w.applyPatch(configPatch{
		namespace: "ns1",
		source:    "cm-a",
		data:      map[string]any{"LOG_LEVEL": "info"},
		revision:  "1",
	})

	w.applyPatch(configPatch{
		namespace: "ns1",
		source:    "cm-b",
		data:      map[string]any{"FEATURE_FLAG": "true"},
		revision:  "2",
	})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to be cached")
	}
	if entry.Sources["cm-a"]["LOG_LEVEL"] != "info" {
		t.Errorf("cm-a data was clobbered: got %+v", entry.Sources["cm-a"])
	}
	if entry.Sources["cm-b"]["FEATURE_FLAG"] != "true" {
		t.Errorf("cm-b data was clobbered: got %+v", entry.Sources["cm-b"])
	}
	if entry.Revision != "2" {
		t.Errorf("Revision: got %q, want latest patch's revision %q", entry.Revision, "2")
	}
}

func TestApplyPatch_ClearOnlyClearsOwnSource(t *testing.T) {
	w := newTestWatcher(t)
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", data: map[string]any{"LOG_LEVEL": "info"}, revision: "1"})
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-b", data: map[string]any{"FEATURE_FLAG": "true"}, revision: "2"})

	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", clearData: true, revision: "3"})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to remain cached (cm-b still present)")
	}
	if _, ok := entry.Sources["cm-a"]; ok {
		t.Errorf("cm-a should have been cleared: got %+v", entry.Sources)
	}
	if entry.Sources["cm-b"]["FEATURE_FLAG"] != "true" {
		t.Errorf("cm-b should be untouched by cm-a's clear: got %+v", entry.Sources["cm-b"])
	}
}

func TestApplyPatch_DeletesEntryWhenAllSourcesCleared(t *testing.T) {
	w := newTestWatcher(t)
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", data: map[string]any{"LOG_LEVEL": "info"}, revision: "1"})

	if w.appCache.Get("ns1") == nil {
		t.Fatal("precondition: expected namespace to be cached")
	}

	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", clearData: true, revision: "2"})

	if w.appCache.Get("ns1") != nil {
		t.Error("expected namespace to be removed once all sources are cleared")
	}
}

func TestApplyPatch_RevisionOnlyUpdatesWhenNonEmpty(t *testing.T) {
	w := newTestWatcher(t)
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", data: map[string]any{"LOG_LEVEL": "info"}, revision: "1"})
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", data: map[string]any{"LOG_LEVEL": "debug"}, revision: ""})

	entry := w.appCache.Get("ns1")
	if entry.Revision != "1" {
		t.Errorf("Revision: got %q, want retained %q", entry.Revision, "1")
	}
}

func TestApplyPatch_UpdatedAtDefaultsToNowWhenZero(t *testing.T) {
	w := newTestWatcher(t)
	before := time.Now().UTC()
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", data: map[string]any{"LOG_LEVEL": "info"}, revision: "1"})
	after := time.Now().UTC()

	entry := w.appCache.Get("ns1")
	if entry.UpdatedAt.Before(before) || entry.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not within [%v, %v]", entry.UpdatedAt, before, after)
	}
}

func TestApplyPatch_UpdatedAtUsesExplicitValue(t *testing.T) {
	w := newTestWatcher(t)
	explicit := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	w.applyPatch(configPatch{namespace: "ns1", source: "cm-a", data: map[string]any{"LOG_LEVEL": "info"}, revision: "1", updated: explicit})

	entry := w.appCache.Get("ns1")
	if !entry.UpdatedAt.Equal(explicit) {
		t.Errorf("UpdatedAt: got %v, want %v", entry.UpdatedAt, explicit)
	}
}

// --- extractObject: direct objects, tombstones, and wrong types ---

func TestExtractObject(t *testing.T) {
	want := &corev1.ConfigMap{Data: map[string]string{"a": "1"}}

	t.Run("direct object", func(t *testing.T) {
		if got := extractObject(want); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("tombstone-wrapped object", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "ns1/cm1", Obj: want}
		if got := extractObject(tomb); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("non-client.Object returns nil", func(t *testing.T) {
		if got := extractObject("not an object"); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("tombstone wrapping non-client.Object returns nil", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "ns1/cm1", Obj: "not an object"}
		if got := extractObject(tomb); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

// --- unwrapTombstone ---

func TestUnwrapTombstone(t *testing.T) {
	t.Run("value tombstone", func(t *testing.T) {
		inner := &corev1.ConfigMap{}
		tomb := cache.DeletedFinalStateUnknown{Key: "ns1/cm1", Obj: inner}
		if got := unwrapTombstone(tomb); got != inner {
			t.Errorf("got %+v, want %+v", got, inner)
		}
	})

	t.Run("pointer tombstone", func(t *testing.T) {
		inner := &corev1.ConfigMap{}
		tomb := &cache.DeletedFinalStateUnknown{Key: "ns1/cm1", Obj: inner}
		if got := unwrapTombstone(tomb); got != inner {
			t.Errorf("got %+v, want %+v", got, inner)
		}
	})

	t.Run("non-tombstone returns nil", func(t *testing.T) {
		if got := unwrapTombstone(&corev1.ConfigMap{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

// --- onSourceUpsert / onSourceDelete ---

func TestOnSourceUpsert(t *testing.T) {
	w := newTestWatcher(t)
	src := NewConfigMapSource(&config.Config{})
	cm := &corev1.ConfigMap{
		ObjectMeta: objectMeta("ns1", "cm1", "1"),
		Data:       map[string]string{"LOG_LEVEL": "info"},
	}

	w.onSourceUpsert(src)(cm)

	entry := w.appCache.Get("ns1")
	if entry == nil || entry.Sources["ConfigMap/cm1"]["LOG_LEVEL"] != "info" {
		t.Fatalf("got %+v", entry)
	}
}

func TestOnSourceUpsert_NonClientObjectIgnored(t *testing.T) {
	w := newTestWatcher(t)
	w.onSourceUpsert(NewConfigMapSource(&config.Config{}))("not an object")

	if w.appCache.Len() != 0 {
		t.Errorf("got %d cached namespaces, want 0", w.appCache.Len())
	}
}

func TestOnSourceDelete_ClearsOwnSourceOnly(t *testing.T) {
	w := newTestWatcher(t)
	src := NewConfigMapSource(&config.Config{})
	upsert := w.onSourceUpsert(src)
	upsert(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "cm-a", "1"), Data: map[string]string{"LOG_LEVEL": "info"}})
	upsert(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "cm-b", "2"), Data: map[string]string{"FEATURE_FLAG": "true"}})

	w.onSourceDelete(src)(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "cm-a", "3")})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to remain cached (cm-b still present)")
	}
	if _, ok := entry.Sources["ConfigMap/cm-a"]; ok {
		t.Errorf("cm-a should have been cleared: got %+v", entry.Sources)
	}
	if entry.Sources["ConfigMap/cm-b"]["FEATURE_FLAG"] != "true" {
		t.Errorf("cm-b should be untouched: got %+v", entry.Sources["ConfigMap/cm-b"])
	}
}

// TestOnSourceUpsert_DifferentKindsDoNotCollide asserts a ConfigMap and an
// unrelated source kind sharing an object name land under distinct keys.
// See TestOnSourceUpsert_SameKindDistinctPrefixDoNotCollide and
// TestOnSourceUpsert_SameKindSamePrefixStillCollides below for the same-Kind
// case, which Kind() alone can't distinguish - only KeyPrefix() can.
func TestOnSourceUpsert_DifferentKindsDoNotCollide(t *testing.T) {
	w := newTestWatcher(t)
	cmSource := NewConfigMapSource(&config.Config{})
	fake := &fakeSource{kind: "Fake", data: map[string]any{"FAKE_KEY": "fake-value"}}

	w.onSourceUpsert(cmSource)(&corev1.ConfigMap{
		ObjectMeta: objectMeta("ns1", "same-name", "1"),
		Data:       map[string]string{"LOG_LEVEL": "info"},
	})
	w.onSourceUpsert(fake)(&corev1.Secret{ObjectMeta: objectMeta("ns1", "same-name", "1")})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to be cached")
	}
	if entry.Sources["ConfigMap/same-name"]["LOG_LEVEL"] != "info" {
		t.Errorf("ConfigMap source missing or clobbered: got %+v", entry.Sources)
	}
	if entry.Sources["Fake/same-name"]["FAKE_KEY"] != "fake-value" {
		t.Errorf("Fake source missing or clobbered: got %+v", entry.Sources)
	}
	if len(entry.Sources) != 2 {
		t.Errorf("expected 2 distinct source keys, got %d: %+v", len(entry.Sources), entry.Sources)
	}
}

// TestOnSourceUpsert_SameKindDistinctPrefixDoNotCollide is the happy path:
// two sources sharing a Kind (as two ConfigMapSources scoped by different
// label selectors would) but registered with distinct KeyPrefix values
// don't clobber each other, even watching an object of the same name in
// the same namespace.
func TestOnSourceUpsert_SameKindDistinctPrefixDoNotCollide(t *testing.T) {
	w := newTestWatcher(t)
	base := &fakeSource{kind: "ConfigMap", keyPrefix: "ConfigMap:base", data: map[string]any{"LOG_LEVEL": "info"}}
	feature := &fakeSource{kind: "ConfigMap", keyPrefix: "ConfigMap:feature", data: map[string]any{"FEATURE_X": "true"}}

	w.onSourceUpsert(base)(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "app-config", "1")})
	w.onSourceUpsert(feature)(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "app-config", "1")})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to be cached")
	}
	if entry.Sources["ConfigMap:base/app-config"]["LOG_LEVEL"] != "info" {
		t.Errorf("base source missing or clobbered: got %+v", entry.Sources)
	}
	if entry.Sources["ConfigMap:feature/app-config"]["FEATURE_X"] != "true" {
		t.Errorf("feature source missing or clobbered: got %+v", entry.Sources)
	}
	if len(entry.Sources) != 2 {
		t.Errorf("expected 2 distinct source keys, got %d: %+v", len(entry.Sources), entry.Sources)
	}

	merged := entry.Merged()
	if merged["LOG_LEVEL"] != "info" || merged["FEATURE_X"] != "true" {
		t.Errorf("expected merged view to contain both sources' keys, got %+v", merged)
	}
}

// TestOnSourceUpsert_SameKindSamePrefixStillCollides is the negative path:
// two sources sharing both a Kind and a KeyPrefix (the default when neither
// source sets one) do still overwrite each other when they watch an object
// of the same name. This isn't a residual bug - KeyPrefix only prevents a
// collision when a source author actually differentiates co-registered
// siblings; it's on the registration, not automatic.
func TestOnSourceUpsert_SameKindSamePrefixStillCollides(t *testing.T) {
	w := newTestWatcher(t)
	first := &fakeSource{kind: "ConfigMap", data: map[string]any{"LOG_LEVEL": "info"}}
	second := &fakeSource{kind: "ConfigMap", data: map[string]any{"FEATURE_X": "true"}}

	w.onSourceUpsert(first)(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "app-config", "1")})
	w.onSourceUpsert(second)(&corev1.ConfigMap{ObjectMeta: objectMeta("ns1", "app-config", "1")})

	entry := w.appCache.Get("ns1")
	if entry == nil {
		t.Fatal("expected namespace to be cached")
	}
	if len(entry.Sources) != 1 {
		t.Fatalf("expected same Kind + same KeyPrefix to collide into 1 source key, got %d: %+v", len(entry.Sources), entry.Sources)
	}
	if entry.Sources["ConfigMap/app-config"]["FEATURE_X"] != "true" {
		t.Errorf("expected second upsert to have overwritten the first, got %+v", entry.Sources)
	}
	if _, stillPresent := entry.Sources["ConfigMap/app-config"]["LOG_LEVEL"]; stillPresent {
		t.Errorf("expected first source's data to be gone, got %+v", entry.Sources)
	}
}

func objectMeta(namespace, name, resourceVersion string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: namespace, Name: name, ResourceVersion: resourceVersion}
}

// --- NewWatcher validation ---

func TestNewWatcher_NoSourcesErrors(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	_, err = NewWatcher(&rest.Config{}, scheme, app.NewCache(), observability.NewMetrics(prometheus.NewRegistry()), nil, nil, zap.NewNop(), nil)
	if err == nil {
		t.Fatal("expected an error for zero registered sources, got nil")
	}
}

func TestNewWatcher_InvalidLabelSelectorErrors(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	sources := []ConfigSource{&fakeSource{kind: "Fake", labelSelector: "!!!not a valid selector!!!"}}
	_, err = NewWatcher(&rest.Config{}, scheme, app.NewCache(), observability.NewMetrics(prometheus.NewRegistry()), nil, nil, zap.NewNop(), sources)
	if err == nil {
		t.Fatal("expected an error for an invalid label selector, got nil")
	}
}

// TestValidateDistinctKeyPrefixes_DuplicateErrors is the fail-fast
// counterpart to the runtime negative-path test in
// TestOnSourceUpsert_SameKindSamePrefixStillCollides - two sources that
// would silently overwrite each other's cache entries fail this check
// instead of colliding at runtime. Wired into NewWatcher so it happens at
// startup, before any informer is registered.
func TestValidateDistinctKeyPrefixes_DuplicateErrors(t *testing.T) {
	sources := []ConfigSource{
		&fakeSource{kind: "ConfigMap"},
		&fakeSource{kind: "ConfigMap"},
	}
	if err := validateDistinctKeyPrefixes(sources); err == nil {
		t.Fatal("expected an error for two sources sharing a KeyPrefix, got nil")
	}
}

// TestValidateDistinctKeyPrefixes_DistinctSucceeds is the happy-path
// counterpart: two sources sharing a Kind but registered with distinct
// KeyPrefix values don't trip the check.
func TestValidateDistinctKeyPrefixes_DistinctSucceeds(t *testing.T) {
	sources := []ConfigSource{
		&fakeSource{kind: "ConfigMap", keyPrefix: "ConfigMap:base"},
		&fakeSource{kind: "ConfigMap", keyPrefix: "ConfigMap:feature"},
	}
	if err := validateDistinctKeyPrefixes(sources); err != nil {
		t.Fatalf("expected distinct KeyPrefix values not to error, got: %v", err)
	}
}

// TestNewWatcher_DuplicateKeyPrefixErrors confirms the check above is
// actually wired into NewWatcher, not just defined and unused.
func TestNewWatcher_DuplicateKeyPrefixErrors(t *testing.T) {
	scheme, err := NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	sources := []ConfigSource{
		&fakeSource{kind: "ConfigMap"},
		&fakeSource{kind: "ConfigMap"},
	}
	_, err = NewWatcher(&rest.Config{}, scheme, app.NewCache(), observability.NewMetrics(prometheus.NewRegistry()), nil, nil, zap.NewNop(), sources)
	if err == nil {
		t.Fatal("expected an error for two sources sharing a KeyPrefix, got nil")
	}
}
