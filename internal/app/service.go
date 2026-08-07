package app

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garoze/muninn/internal/config"
	"go.uber.org/zap"
)

// ConfigEntry holds cached configuration for a namespace. Sources is keyed
// by contributing object name; each source owns its own slice so one
// source's update can't clobber another's (see docs/design.md).
type ConfigEntry struct {
	Namespace string
	Sources   map[string]map[string]any
	Revision  string
	UpdatedAt time.Time
}

// Merged returns the flattened view of all sources' data. On key collision,
// the alphabetically later source name wins (deterministic, not
// semantically meaningful).
func (e *ConfigEntry) Merged() map[string]any {
	out := make(map[string]any)
	for _, name := range sortedSourceNames(e.Sources) {
		maps.Copy(out, e.Sources[name])
	}
	return out
}

// Resolve returns a key's value and which source contributed it, following
// the same precedence as Merged.
func (e *ConfigEntry) Resolve(key string) (value any, source string, ok bool) {
	for _, name := range sortedSourceNames(e.Sources) {
		if v, exists := e.Sources[name][key]; exists {
			value, source, ok = v, name, true
		}
	}
	return
}

func sortedSourceNames(sources map[string]map[string]any) []string {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Cache holds in-memory configuration state keyed by namespace, guarded by
// an RWMutex. Marked synced once the informer completes its initial
// list+watch cycle.
type Cache struct {
	mu          sync.RWMutex
	byNamespace map[string]*ConfigEntry
	synced      atomic.Bool
}

// NewCache creates an empty config cache.
func NewCache() *Cache {
	return &Cache{
		byNamespace: make(map[string]*ConfigEntry),
	}
}

// IsSynced reports whether the cache has completed initial sync.
func (c *Cache) IsSynced() bool {
	return c.synced.Load()
}

// SetSynced marks the cache as ready to serve traffic.
func (c *Cache) SetSynced() {
	c.synced.Store(true)
}

// Get returns the entry for a namespace, or nil if not found.
func (c *Cache) Get(namespace string) *ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byNamespace[namespace]
}

// Set stores or replaces a namespace's entry under a write lock.
func (c *Cache) Set(entry *ConfigEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byNamespace[entry.Namespace] = entry
}

// Delete removes a namespace from the cache.
func (c *Cache) Delete(namespace string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byNamespace, namespace)
}

// Apply mutates a namespace's entry under a single write lock. mutate receives
// the current entry (nil when absent) and returns the entry to store, or nil to
// delete it. Separate Get and Set calls would let two sources' concurrent
// read-modify-write interleave and drop one source's contribution entirely.
//
// mutate must not call back into Cache: the write lock is held for its
// duration and sync.RWMutex is not reentrant.
func (c *Cache) Apply(namespace string, mutate func(current *ConfigEntry) *ConfigEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	next := mutate(c.byNamespace[namespace])
	if next == nil {
		delete(c.byNamespace, namespace)
		return
	}
	c.byNamespace[namespace] = next
}

// Len returns the number of cached namespaces.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.byNamespace)
}

// QueryResult represents a single resolved key-value pair.
type QueryResult struct {
	Key    string
	Value  any
	Source string // which config source (e.g. ConfigMap name) this was resolved from.
}

// ConfigSourceDescriptor describes an active config source (kind, label
// selector, scope) for the Describe RPC — the only part of a source's shape
// that's static, since the underlying data has no fixed key vocabulary.
type ConfigSourceDescriptor struct {
	Kind          string
	LabelSelector string
	Scope         string
}

// DiscoveryService implements the core query logic.
type DiscoveryService struct {
	Cache         *Cache
	log           *zap.Logger
	cacheEntryTTL time.Duration
	now           func() time.Time
	sources       []ConfigSourceDescriptor
}

// NewDiscoveryService constructs a DiscoveryService with an empty cache.
// sources comes from the caller so this package stays free of Kubernetes
// dependencies (see docs/design.md).
func NewDiscoveryService(cfg *config.Config, log *zap.Logger, sources []ConfigSourceDescriptor) *DiscoveryService {
	return &DiscoveryService{
		Cache:         NewCache(),
		log:           log,
		cacheEntryTTL: cfg.CacheEntryTTL,
		now:           time.Now,
		sources:       sources,
	}
}

// Describe returns the active config sources, for the Describe RPC.
func (s *DiscoveryService) Describe() []ConfigSourceDescriptor {
	return s.sources
}

// Query resolves the requested keys from the namespace's cached configuration.
//
// Error Precedence:
//   - cache not synced		-> ErrCacheNotSynced	(Unavailable)
//   - namespace empty		-> ErrNamespaceRequired	(InvalidArgument)
//   - namespace not found	-> ErrNamespaceNotFound	(NotFound)
//   - stale entry			-> ErrCacheEntryStale	(Unavailable)
//   - strict + missing key	-> ErrStrictMissingKeys (InvalidArgument)
func (s *DiscoveryService) Query(ctx context.Context, namespace string, keys []string, strict bool) ([]QueryResult, []string, string, error) {
	if !s.Cache.IsSynced() {
		return nil, nil, "", ErrCacheNotSynced
	}

	if namespace == "" {
		return nil, nil, "", ErrNamespaceRequired
	}

	entry := s.Cache.Get(namespace)
	if entry == nil {
		return nil, nil, "", fmt.Errorf("%w: %s", ErrNamespaceNotFound, namespace)
	}

	if s.cacheEntryTTL > 0 && !entry.UpdatedAt.IsZero() {
		if s.now().Sub(entry.UpdatedAt) > s.cacheEntryTTL {
			return nil, nil, "", fmt.Errorf("%w: %s", ErrCacheEntryStale, namespace)
		}
	}

	var results []QueryResult
	var missing []string

	for _, k := range keys {
		v, source, ok := entry.Resolve(k)
		if !ok {
			missing = append(missing, k)
			continue
		}
		results = append(results, QueryResult{Key: k, Value: v, Source: source})
	}

	if strict && len(missing) > 0 {
		return nil, nil, "", fmt.Errorf("%w: %s", ErrStrictMissingKeys, missing)
	}

	s.log.Debug("query completed",
		zap.String("namespace", namespace),
		zap.Int("keys_requested", len(keys)),
		zap.Int("keys_resolved", len(results)),
	)

	return results, missing, entry.Revision, nil
}

// Resolve returns every key currently resolved for a namespace, without the
// caller enumerating keys up front — the counterpart to Query for callers
// that want everything rather than specific keys.
//
// Error Precedence: same as Query, minus strict/missing-keys since there's
// nothing to be missing.
func (s *DiscoveryService) Resolve(ctx context.Context, namespace string) ([]QueryResult, string, error) {
	if !s.Cache.IsSynced() {
		return nil, "", ErrCacheNotSynced
	}

	if namespace == "" {
		return nil, "", ErrNamespaceRequired
	}

	entry := s.Cache.Get(namespace)
	if entry == nil {
		return nil, "", fmt.Errorf("%w: %s", ErrNamespaceNotFound, namespace)
	}

	if s.cacheEntryTTL > 0 && !entry.UpdatedAt.IsZero() {
		if s.now().Sub(entry.UpdatedAt) > s.cacheEntryTTL {
			return nil, "", fmt.Errorf("%w: %s", ErrCacheEntryStale, namespace)
		}
	}

	results := make([]QueryResult, 0)
	index := make(map[string]int) // key -> index into results, so later source can overwrite in place

	for _, name := range sortedSourceNames(entry.Sources) {
		for k, v := range entry.Sources[name] {
			if i, ok := index[k]; ok {
				results[i] = QueryResult{Key: k, Value: v, Source: name}
				continue
			}
			index[k] = len(results)
			results = append(results, QueryResult{Key: k, Value: v, Source: name})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Key < results[j].Key })

	s.log.Debug("resolve completed",
		zap.String("namespace", namespace),
		zap.Int("keys_resolved", len(results)),
	)

	return results, entry.Revision, nil
}
