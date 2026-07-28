package app

import "errors"

var (
	// ErrCacheNotSynced is returned when the informer cache has not yet
	// completed its initial list+watch cycle.
	ErrCacheNotSynced = errors.New("cache not synced")

	// ErrTenantNotFound is returned when the requested tenant has no entry
	// in the cache.
	ErrTenantNotFound = errors.New("tenant not found")

	// ErrUnsupportedKey is returned when a requested key is not in SupportedKeys.
	ErrUnsupportedKey = errors.New("unsupported key")

	// ErrTenantIDRequired is returned when tenant_id is empty in the request.
	ErrTenantIDRequired = errors.New("tenant_id is required")

	// ErrStrictMissingKeys is returned in strict mode when one or mode
	// requested keys are absent from the cache entry.
	ErrStrictMissingKeys = errors.New("strict mode: missing keys")

	// ErrCacheEntryStale is returned when the cache entry has exceeded
	// CacheEntryTTL and staleness enforcement is enabled.
	ErrCacheEntryStale = errors.New("cache entry is stale")
)
