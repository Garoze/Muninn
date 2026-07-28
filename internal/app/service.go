package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// SupportedKeys is the whitelisted key namespace exposed by the Query API.
// Keys are dot-separated. Anything not in this map returns InvalidArgument.
var SupportedKeys = map[string]string{
	"TENANT.id":                          "string",
	"TENANT.displayName":                 "string",
	"TENANT.runtime":                     "object",
	"TENANT.resources.identityPoolID":    "string",
	"TENANT.resources.identityPoolARN":   "string",
	"TENANT.resources.storageBucketName": "string",
	"AUTHZ.jwt.issuerAllowList":          "array",
	"AUTHZ.jwt.subjectClaim":             "string",
	"AUTHZ.jwt.scopesClaim":              "string",
	"AUTHZ.bindings":                     "array",
	"AUTHZ.roleBindings":                 "array",
}

// SupportedKeyDescriptions provides human-readable descriptions for the Describe RPC.
var SupportedKeyDescriptions = map[string]string{
	"TENANT.id":                          "Stable tenant identifier",
	"TENANT.displayName":                 "Human-readable display name",
	"TENANT.runtime":                     "Runtime configuration bag (non-secret)",
	"TENANT.resources.identityPoolID":    "Provisioned identity pool ID",
	"TENANT.resources.identityPoolARN":   "Provisioned identity pool ARN",
	"TENANT.resources.storageBucketName": "Provisioned storage bucket name",
	"AUTHZ.jwt.issuerAllowList":          "Accepted JWT issuers (empty = skip validation)",
	"AUTHZ.jwt.subjectClaim":             "JWT claim used as subject (default: sub)",
	"AUTHZ.jwt.scopesClaim":              "JWT claim used for scopes (default: scp)",
	"AUTHZ.bindings":                     "Subject-to-permissions bindings",
	"AUTHZ.roleBindings":                 "Role-to-permissions bindings",
}

// AuthzBindingSnapshot is the in-memory representation of a subject binding.
type AuthzBindingSnapshot struct {
	Subject     string
	Permissions []string
}

// AuthzRoleBindingSnapshot is the in-memory representation of a role binding.
type AuthzRoleBindingSnapshot struct {
	Role        string
	Permissions []string
}

// AuthzPolicySnapshot is the in-memory projection of a tenant's Policy spec.
type AuthzPolicySnapshot struct {
	IssuerAllowList []string
	SubjectClaim    string
	ScopesClaim     string
	Bindings        []AuthzBindingSnapshot
	RoleBindings    []AuthzRoleBindingSnapshot
}

// TenantState holds the extracted, cached data for a dingle tenant.
// Fields are populated by the informer handlers in internal/kube/watcher.go
type TenantState struct {
	TenantID       string
	DisplayName    string
	RuntimeConfig  map[string]any
	CloudResources map[string]any
	AuthzPolicy    *AuthzPolicySnapshot
	Revision       string
	UpdatedAt      time.Time
}

// Cache holds in-memory tenant state keyed bu tenant ID.
// Reads use Rlock; writes use full Lock. Marked synced after
// the informer cache completes its initial list+watch cycle.
type Cache struct {
	mu       sync.RWMutex
	byTenant map[string]*TenantState
	synced   atomic.Bool
}

// NewCache creates an empty tenant cache.
func NewCache() *Cache {
	return &Cache{
		byTenant: make(map[string]*TenantState),
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

// Get returns the state for a tenant, or nil if not found.
func (c *Cache) Get(tenantID string) *TenantState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byTenant[tenantID]
}

// Set stores or replaces tenant state under a write lock.
func (c *Cache) Set(state *TenantState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byTenant[state.TenantID] = state
}

// Delete removes a tenant from the cache
func (c *Cache) Delete(tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byTenant, tenantID)
}

// Len returns the numer of cached tenants.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.byTenant)
}

// QueryResult represents a single resolved key-valu pair.
type QueryResult struct {
	Key    string
	Value  any
	Source string // which CRD field this was resolved from.
}

// DiscoveryService implements the core query logic.
type DiscoveryService struct {
	Cache         *Cache
	Logger        *zap.Logger
	CacheEntryTTL time.Duration
	now           func() time.Time
}

// NewDiscoveryService contructs a DiscoveryService with an empty cache.
func NewDiscoveryService(logger *zap.Logger) *DiscoveryService {
	return &DiscoveryService{
		Cache:  NewCache(),
		Logger: logger,
		now:    time.Now,
	}
}

// Query resolves the requested keys from the tenant's cached state.
//
// Error Precedence:
//   - cache not sunced		-> ErrCacheNotSynced	(Unavailable)
//   - tenant_id empty		-> ErrTenantIDRequired	(InvalidArgument)
//   - unsupported key		-> ErrUnsupportedKey	(invalidArgument)
//   - tenant not fud		-> ErrTenantNotFound	(NotFound)
//   - stale entry			-> ErrCacheEntryStale	(Unavailable)
//   - strict + missing key	-> ErrStrictMissingKeys (InvalidArgument)
func (s *DiscoveryService) Query(ctx context.Context, tenantID string, keys []string, strict bool) ([]QueryResult, []string, string, error) {
	if !s.Cache.IsSynced() {
		return nil, nil, "", fmt.Errorf("%w", ErrCacheNotSynced)
	}

	if tenantID == "" {
		return nil, nil, "", fmt.Errorf("%w", ErrTenantIDRequired)
	}

	for _, k := range keys {
		if _, ok := SupportedKeys[k]; !ok {
			return nil, nil, "", fmt.Errorf("%w: %s", ErrUnsupportedKey, k)
		}
	}

	// Cache keys are bare tenant IDs; strip the namespace prefix if present.
	normalizedID := strings.TrimPrefix(tenantID, "tenant-")
	state := s.Cache.Get(normalizedID)
	if state == nil {
		return nil, nil, "", fmt.Errorf("%w: %s", ErrTenantNotFound, tenantID)
	}

	if s.CacheEntryTTL > 0 && !state.UpdatedAt.IsZero() {
		if s.now().Sub(state.UpdatedAt) > s.CacheEntryTTL {
			return nil, nil, "", fmt.Errorf("%w: %s", ErrCacheEntryStale, tenantID)
		}
	}

	var results []QueryResult
	var missing []string

	for _, k := range keys {
		switch k {

		case "TENANT.id":
			results = append(results, QueryResult{Key: k, Value: state.TenantID, Source: "tenant.spec.tenantID"})

		case "TENANT.displayName":
			results = append(results, QueryResult{Key: k, Value: state.DisplayName, Source: "tenant.spec.displayName"})

		case "TENANT.runtime":
			if state.RuntimeConfig == nil {
				missing = append(missing, k)
				continue
			}

			results = append(results, QueryResult{Key: k, Value: state.RuntimeConfig, Source: "tenant.spec.runtimeConfig"})

		case "TENANT.resources.identityPoolID":
			v, ok := CloudResourceField(state.CloudResources, "identityPoolID")
			if !ok {
				missing = append(missing, k)
				continue
			}

			results = append(results, QueryResult{Key: k, Value: v, Source: "tenant.status.cloudResources.identityPoolID"})

		case "TENANT.resources.identityPoolARN":
			v, ok := CloudResourceField(state.CloudResources, "identityPoolARN")
			if !ok {
				missing = append(missing, k)
				continue
			}

			results = append(results, QueryResult{Key: k, Value: v, Source: "tenant.status.cloudResources.identityPoolARN"})

		case "TENANT.resources.storageBucketName":
			v, ok := CloudResourceField(state.CloudResources, "storageBucketName")
			if !ok {
				missing = append(missing, k)
				continue
			}

			results = append(results, QueryResult{Key: k, Value: v, Source: "tenant.status.cloudResources.storageBucketName"})

		case "AUTHZ.jwt.issuerAllowList":
			if state.AuthzPolicy == nil {
				missing = append(missing, k)
				continue
			}

			issuers := make([]any, 0, len(state.AuthzPolicy.IssuerAllowList))
			for _, issuer := range state.AuthzPolicy.IssuerAllowList {
				issuers = append(issuers, issuer)
			}

			results = append(results, QueryResult{Key: k, Value: issuers, Source: "policy.spec.jwt.issuerAllowList"})

		case "AUTHZ.jwt.subjectClaim":
			if state.AuthzPolicy == nil {
				missing = append(missing, k)
				continue
			}

			results = append(results, QueryResult{Key: k, Value: state.AuthzPolicy.SubjectClaim, Source: "policy.spec.jwt.subjectClaim"})

		case "AUTHZ.jwt.scopesClaim":
			if state.AuthzPolicy == nil {
				missing = append(missing, k)
				continue
			}

			results = append(results, QueryResult{Key: k, Value: state.AuthzPolicy.ScopesClaim, Source: "policy.spec.jwt.scopesClaim"})

		case "AUTHZ.bindings":
			if state.AuthzPolicy == nil {
				missing = append(missing, k)
				continue
			}

			bindings := make([]any, 0, len(state.AuthzPolicy.Bindings))
			for _, b := range state.AuthzPolicy.Bindings {
				perms := make([]any, 0, len(b.Permissions))
				for _, p := range b.Permissions {
					perms = append(perms, p)
				}
				bindings = append(bindings, map[string]any{"subject": b.Subject, "permissions": perms})
			}

			results = append(results, QueryResult{Key: k, Value: bindings, Source: "policy.spec.bindings"})

		case "AUTHZ.roleBindings":
			if state.AuthzPolicy == nil {
				missing = append(missing, k)
				continue
			}

			roleBindings := make([]any, 0, len(state.AuthzPolicy.RoleBindings))
			for _, rb := range state.AuthzPolicy.RoleBindings {
				perms := make([]any, 0, len(rb.Permissions))
				for _, p := range rb.Permissions {
					perms = append(perms, p)
				}
				roleBindings = append(roleBindings, map[string]any{"role": rb.Role, "permissions": perms})
			}

			results = append(results, QueryResult{Key: k, Value: roleBindings, Source: "policy.spec.roleBindings"})

		default:
			missing = append(missing, k)

		}
	}

	if strict && len(missing) > 0 {
		return nil, nil, "", fmt.Errorf("%w: %s", ErrStrictMissingKeys, missing)
	}

	s.Logger.Debug("query completed",
		zap.String("tenant_id", tenantID),
		zap.Int("keys_requested", len(keys)),
		zap.Int("keys_resolved", len(results)),
	)

	return results, missing, state.Revision, nil
}

// CloudResourceField extracts a named string field from the CloudResources map.
func CloudResourceField(resources map[string]any, field string) (string, bool) {
	if resources == nil {
		return "", false
	}

	v, ok := resources[field]
	if !ok {
		return "", false
	}

	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}

	return s, true
}
