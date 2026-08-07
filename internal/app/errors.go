package app

import "errors"

var (
	// ErrCacheNotSynced is returned when the informer cache has not yet
	// completed its initial list+watch cycle.
	ErrCacheNotSynced = errors.New("cache not synced")

	// ErrNamespaceNotFound is returned when the requested namespace has no
	// entry in the cache.
	ErrNamespaceNotFound = errors.New("namespace not found")

	// ErrNamespaceRequired is returned when namespace is empty in the request.
	ErrNamespaceRequired = errors.New("namespace is required")

	// ErrStrictMissingKeys is returned in strict mode when one or more
	// requested keys are absent from the cache entry.
	ErrStrictMissingKeys = errors.New("strict mode: missing keys")

	// ErrCacheEntryStale is returned when the cache entry has exceeded
	// CacheEntryTTL and staleness enforcement is enabled.
	ErrCacheEntryStale = errors.New("cache entry is stale")
)
