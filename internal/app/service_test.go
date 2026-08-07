package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garoze/muninn/internal/config"
	"go.uber.org/zap"
)

func newTestService(t *testing.T) *DiscoveryService {
	t.Helper()
	return &DiscoveryService{
		Cache: NewCache(),
		log:   zap.NewNop(),
		now:   time.Now,
	}
}

// --- Cache ---

func TestCache_GetMiss(t *testing.T) {
	c := NewCache()
	if got := c.Get("missing"); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestCache_SetGetRoundtrip(t *testing.T) {
	c := NewCache()
	entry := &ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"KEY": "value"}}}
	c.Set(entry)

	got := c.Get("ns1")
	if got == nil {
		t.Fatal("got nil, want entry")
	}
	if got.Sources["cm1"]["KEY"] != "value" {
		t.Errorf("got %+v, want KEY=value", got)
	}
}

func TestCache_SetOverwrites(t *testing.T) {
	c := NewCache()
	c.Set(&ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"KEY": "first"}}})
	c.Set(&ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"KEY": "second"}}})

	got := c.Get("ns1")
	if got.Sources["cm1"]["KEY"] != "second" {
		t.Errorf("got %+v, want KEY=second", got)
	}
	if c.Len() != 1 {
		t.Errorf("Len: got %d, want 1", c.Len())
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache()
	c.Set(&ConfigEntry{Namespace: "ns1"})
	c.Delete("ns1")

	if got := c.Get("ns1"); got != nil {
		t.Errorf("got %+v, want nil after delete", got)
	}
}

func TestCache_DeleteNonExistentNoop(t *testing.T) {
	c := NewCache()
	c.Delete("missing") // must not panic
	if c.Len() != 0 {
		t.Errorf("Len: got %d, want 0", c.Len())
	}
}

func TestCache_ApplyCreatesWhenAbsent(t *testing.T) {
	c := NewCache()

	var sawCurrent *ConfigEntry
	c.Apply("ns1", func(current *ConfigEntry) *ConfigEntry {
		sawCurrent = current
		return &ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"KEY": "value"}}}
	})

	if sawCurrent != nil {
		t.Errorf("mutate should see nil for an absent namespace, got %+v", sawCurrent)
	}
	if got := c.Get("ns1"); got == nil || got.Sources["cm1"]["KEY"] != "value" {
		t.Errorf("got %+v, want the entry mutate returned", got)
	}
}

func TestCache_ApplySeesCurrentEntry(t *testing.T) {
	c := NewCache()
	c.Set(&ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"KEY": "first"}}})

	c.Apply("ns1", func(current *ConfigEntry) *ConfigEntry {
		if current == nil || current.Sources["cm1"]["KEY"] != "first" {
			t.Fatalf("mutate should see the stored entry, got %+v", current)
		}
		return &ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"KEY": "second"}}}
	})

	if got := c.Get("ns1"); got.Sources["cm1"]["KEY"] != "second" {
		t.Errorf("got %+v, want the replacement entry", got)
	}
}

func TestCache_ApplyNilResultDeletes(t *testing.T) {
	c := NewCache()
	c.Set(&ConfigEntry{Namespace: "ns1"})

	c.Apply("ns1", func(*ConfigEntry) *ConfigEntry { return nil })

	if got := c.Get("ns1"); got != nil {
		t.Errorf("got %+v, want nil after mutate returned nil", got)
	}
}

func TestCache_Len(t *testing.T) {
	c := NewCache()
	if c.Len() != 0 {
		t.Errorf("Len: got %d, want 0", c.Len())
	}
	c.Set(&ConfigEntry{Namespace: "ns1"})
	c.Set(&ConfigEntry{Namespace: "ns2"})
	if c.Len() != 2 {
		t.Errorf("Len: got %d, want 2", c.Len())
	}
}

func TestCache_SyncedDefaultsFalse(t *testing.T) {
	c := NewCache()
	if c.IsSynced() {
		t.Error("IsSynced: got true, want false before SetSynced")
	}
}

func TestCache_SetSynced(t *testing.T) {
	c := NewCache()
	c.SetSynced()
	if !c.IsSynced() {
		t.Error("IsSynced: got false, want true after SetSynced")
	}
}

// --- ConfigEntry.Merged / Resolve ---

func TestConfigEntry_MergedCombinesAllSources(t *testing.T) {
	e := &ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"LOG_LEVEL": "info"},
			"cm-b": {"FEATURE_FLAG": "true"},
		},
	}

	merged := e.Merged()
	if merged["LOG_LEVEL"] != "info" || merged["FEATURE_FLAG"] != "true" {
		t.Errorf("got %+v", merged)
	}
}

func TestConfigEntry_MergedCollisionLaterSourceWins(t *testing.T) {
	e := &ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"LOG_LEVEL": "info"},
			"cm-z": {"LOG_LEVEL": "debug"},
		},
	}

	if got := e.Merged()["LOG_LEVEL"]; got != "debug" {
		t.Errorf("got %v, want debug (cm-z sorts after cm-a)", got)
	}
}

func TestConfigEntry_ResolveReturnsSourceName(t *testing.T) {
	e := &ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"LOG_LEVEL": "info"},
		},
	}

	v, source, ok := e.Resolve("LOG_LEVEL")
	if !ok || v != "info" || source != "cm-a" {
		t.Errorf("got (%v, %q, %v), want (info, cm-a, true)", v, source, ok)
	}
}

func TestConfigEntry_ResolveMissingKey(t *testing.T) {
	e := &ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm-a": {"LOG_LEVEL": "info"}}}

	if _, _, ok := e.Resolve("NOT_PRESENT"); ok {
		t.Error("got ok=true, want false for missing key")
	}
}

// --- DiscoveryService.Describe ---

func TestDescribe_ReturnsConfiguredSource(t *testing.T) {
	want := []ConfigSourceDescriptor{
		{Kind: "ConfigMap", LabelSelector: "muninn.io/config=runtime", Scope: "namespace"},
	}
	svc := NewDiscoveryService(&config.Config{}, zap.NewNop(), want)

	sources := svc.Describe()
	if len(sources) != 1 || sources[0] != want[0] {
		t.Errorf("got %+v, want %+v", sources, want)
	}
}

// --- DiscoveryService.Query: error precedence ---

func TestQuery_CacheNotSynced(t *testing.T) {
	svc := newTestService(t)
	// Cache deliberately left unsynced.

	_, _, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL"}, false)
	if !errors.Is(err, ErrCacheNotSynced) {
		t.Fatalf("got %v, want ErrCacheNotSynced", err)
	}
}

func TestQuery_NamespaceRequired(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()

	_, _, _, err := svc.Query(context.Background(), "", []string{"LOG_LEVEL"}, false)
	if !errors.Is(err, ErrNamespaceRequired) {
		t.Fatalf("got %v, want ErrNamespaceRequired", err)
	}
}

func TestQuery_NamespaceNotFound(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()

	_, _, _, err := svc.Query(context.Background(), "missing-ns", []string{"LOG_LEVEL"}, false)
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("got %v, want ErrNamespaceNotFound", err)
	}
}

func TestQuery_StaleCacheEntry(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.cacheEntryTTL = time.Minute
	svc.now = func() time.Time { return time.Unix(1000, 0) }
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm1": {"LOG_LEVEL": "info"}},
		Revision:  "1",
		UpdatedAt: time.Unix(1000, 0).Add(-2 * time.Minute),
	})

	_, _, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL"}, false)
	if !errors.Is(err, ErrCacheEntryStale) {
		t.Fatalf("got %v, want ErrCacheEntryStale", err)
	}
}

func TestQuery_TTLZeroDisablesStaleCheck(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.cacheEntryTTL = 0 // disabled
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm1": {"LOG_LEVEL": "info"}},
		Revision:  "1",
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	})

	_, _, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL"}, false)
	if err != nil {
		t.Fatalf("unexpected error with TTL disabled: %v", err)
	}
}

func TestQuery_FreshEntryWithinTTL(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.cacheEntryTTL = time.Minute
	svc.now = func() time.Time { return time.Unix(1000, 0) }
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm1": {"LOG_LEVEL": "info"}},
		Revision:  "1",
		UpdatedAt: time.Unix(1000, 0).Add(-30 * time.Second),
	})

	_, _, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL"}, false)
	if err != nil {
		t.Fatalf("unexpected error for fresh entry: %v", err)
	}
}

func TestQuery_StrictMissingKeys(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {}}, Revision: "1"})

	_, _, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL"}, true)
	if !errors.Is(err, ErrStrictMissingKeys) {
		t.Fatalf("got %v, want ErrStrictMissingKeys", err)
	}
}

func TestQuery_NonStrictMissingKeysReturnsListNoError(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {}}, Revision: "1"})

	results, missing, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results: got %+v, want empty", results)
	}
	if len(missing) != 1 || missing[0] != "LOG_LEVEL" {
		t.Errorf("missing: got %+v, want [LOG_LEVEL]", missing)
	}
}

// --- DiscoveryService.Query: success paths ---

func TestQuery_ResolvesKeysFromMergedSources(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"runtime-config": {"LOG_LEVEL": "info", "FEATURE_DARKMODE": "true"},
		},
		Revision: "42",
	})

	results, missing, revision, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL", "FEATURE_DARKMODE"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing: got %+v, want none", missing)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if revision != "42" {
		t.Errorf("revision: got %q, want %q", revision, "42")
	}
	for _, r := range results {
		if r.Source != "runtime-config" {
			t.Errorf("source: got %q, want runtime-config", r.Source)
		}
	}
}

func TestQuery_ResolvesAcrossMultipleSources(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"LOG_LEVEL": "info"},
			"cm-b": {"FEATURE_FLAG": "true"},
		},
		Revision: "1",
	})

	results, _, _, err := svc.Query(context.Background(), "ns1", []string{"LOG_LEVEL", "FEATURE_FLAG"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bySource := map[string]string{}
	for _, r := range results {
		bySource[r.Key] = r.Source
	}
	if bySource["LOG_LEVEL"] != "cm-a" || bySource["FEATURE_FLAG"] != "cm-b" {
		t.Errorf("got %+v", bySource)
	}
}

// --- DiscoveryService.Resolve ---
//
// Resolve deliberately duplicates Query's cache-lookup precedence rather than
// sharing a helper (see docs/adr/0011-resolve-rpc.md). The duplicate needs its
// own coverage: it is what the admission webhook and the injected containers
// read, so an inverted precedence here would reach every consumer's config file
// while every Query test stayed green.

func TestResolve_ReturnsEveryKeyWithoutEnumeratingThem(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm-a": {"LOG_LEVEL": "info", "TIMEOUT": "30s"}},
		Revision:  "7",
	})

	results, revision, err := svc.Resolve(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want every key in the namespace: %+v", len(results), results)
	}
	if revision != "7" {
		t.Errorf("revision: got %q, want %q", revision, "7")
	}
}

func TestResolve_MergesAcrossSourcesAndAttributesEach(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"LOG_LEVEL": "info"},
			"cm-b": {"FEATURE_FLAG": "true"},
		},
	})

	results, _, err := svc.Resolve(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bySource := map[string]string{}
	for _, r := range results {
		bySource[r.Key] = r.Source
	}
	if bySource["LOG_LEVEL"] != "cm-a" || bySource["FEATURE_FLAG"] != "cm-b" {
		t.Errorf("each key must be attributed to the source that contributed it, got %+v", bySource)
	}
}

// The precedence has to match Merged and Query: on a key collision the
// alphabetically later source name wins.
func TestResolve_CollisionLaterSourceWins(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"LOG_LEVEL": "info"},
			"cm-b": {"LOG_LEVEL": "debug"},
		},
	})

	results, _, err := svc.Resolve(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("a colliding key must appear once, got %+v", results)
	}
	if results[0].Value != "debug" || results[0].Source != "cm-b" {
		t.Errorf("got %+v, want cm-b's debug (alphabetically later source wins)", results[0])
	}

	entry := svc.Cache.Get("ns1")
	if got := entry.Merged()["LOG_LEVEL"]; got != results[0].Value {
		t.Errorf("Resolve disagrees with Merged on precedence: %v vs %v", results[0].Value, got)
	}
}

func TestResolve_ResultsSortedByKey(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources: map[string]map[string]any{
			"cm-a": {"ZEBRA": "1", "ALPHA": "2"},
			"cm-b": {"MIDDLE": "3"},
		},
	})

	results, _, err := svc.Resolve(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var keys []string
	for _, r := range results {
		keys = append(keys, r.Key)
	}
	want := []string{"ALPHA", "MIDDLE", "ZEBRA"}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("results must be sorted by key: got %v, want %v", keys, want)
		}
	}
}

func TestResolve_CacheNotSynced(t *testing.T) {
	svc := newTestService(t)
	if _, _, err := svc.Resolve(context.Background(), "ns1"); !errors.Is(err, ErrCacheNotSynced) {
		t.Errorf("got %v, want ErrCacheNotSynced", err)
	}
}

func TestResolve_NamespaceRequired(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	if _, _, err := svc.Resolve(context.Background(), ""); !errors.Is(err, ErrNamespaceRequired) {
		t.Errorf("got %v, want ErrNamespaceRequired", err)
	}
}

func TestResolve_NamespaceNotFound(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	if _, _, err := svc.Resolve(context.Background(), "missing"); !errors.Is(err, ErrNamespaceNotFound) {
		t.Errorf("got %v, want ErrNamespaceNotFound", err)
	}
}

func TestResolve_StaleCacheEntry(t *testing.T) {
	svc := newTestService(t)
	svc.cacheEntryTTL = time.Minute
	svc.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	svc.Cache.SetSynced()
	svc.Cache.Set(&ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm-a": {"LOG_LEVEL": "info"}},
		UpdatedAt: time.Now(),
	})

	if _, _, err := svc.Resolve(context.Background(), "ns1"); !errors.Is(err, ErrCacheEntryStale) {
		t.Errorf("got %v, want ErrCacheEntryStale", err)
	}
}
