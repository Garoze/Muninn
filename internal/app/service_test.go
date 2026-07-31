package app

import (
	"context"
	"errors"
	"testing"
	"time"

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
	state := &TenantState{TenantID: "t1", DisplayName: "Test"}
	c.Set(state)

	got := c.Get("t1")
	if got == nil {
		t.Fatal("got nil, want state")
	}
	if got.DisplayName != "Test" {
		t.Errorf("DisplayName: got %q, want %q", got.DisplayName, "Test")
	}
}

func TestCache_SetOverwrites(t *testing.T) {
	c := NewCache()
	c.Set(&TenantState{TenantID: "t1", DisplayName: "First"})
	c.Set(&TenantState{TenantID: "t1", DisplayName: "Second"})

	got := c.Get("t1")
	if got.DisplayName != "Second" {
		t.Errorf("DisplayName: got %q, want %q", got.DisplayName, "Second")
	}
	if c.Len() != 1 {
		t.Errorf("Len: got %d, want 1", c.Len())
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache()
	c.Set(&TenantState{TenantID: "t1"})
	c.Delete("t1")

	if got := c.Get("t1"); got != nil {
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

func TestCache_Len(t *testing.T) {
	c := NewCache()
	if c.Len() != 0 {
		t.Errorf("Len: got %d, want 0", c.Len())
	}
	c.Set(&TenantState{TenantID: "t1"})
	c.Set(&TenantState{TenantID: "t2"})
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

// --- CloudResourceField ---

func TestCloudResourceField(t *testing.T) {
	tests := []struct {
		name      string
		resources map[string]any
		field     string
		wantVal   string
		wantOK    bool
	}{
		{"nil map", nil, "identityPoolID", "", false},
		{"missing field", map[string]any{"other": "x"}, "identityPoolID", "", false},
		{"non-string value", map[string]any{"identityPoolID": 42}, "identityPoolID", "", false},
		{"empty string value", map[string]any{"identityPoolID": ""}, "identityPoolID", "", false},
		{"valid string", map[string]any{"identityPoolID": "pool-1"}, "identityPoolID", "pool-1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOK := CloudResourceField(tt.resources, tt.field)
			if gotVal != tt.wantVal || gotOK != tt.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", gotVal, gotOK, tt.wantVal, tt.wantOK)
			}
		})
	}
}

// --- DiscoveryService.Query: error precedence ---

func TestQuery_CacheNotSynced(t *testing.T) {
	svc := newTestService(t)
	// Cache deliberately left unsynced.

	_, _, _, err := svc.Query(context.Background(), "t1", []string{"TENANT.id"}, false)
	if !errors.Is(err, ErrCacheNotSynced) {
		t.Fatalf("got %v, want ErrCacheNotSynced", err)
	}
}

func TestQuery_TenantIDRequired(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()

	_, _, _, err := svc.Query(context.Background(), "", []string{"TENANT.id"}, false)
	if !errors.Is(err, ErrTenantIDRequired) {
		t.Fatalf("got %v, want ErrTenantIDRequired", err)
	}
}

func TestQuery_UnsupportedKey(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()

	_, _, _, err := svc.Query(context.Background(), "t1", []string{"NOT.a.real.key"}, false)
	if !errors.Is(err, ErrUnsupportedKey) {
		t.Fatalf("got %v, want ErrUnsupportedKey", err)
	}
}

func TestQuery_UnsupportedKeyCheckedBeforeTenantLookup(t *testing.T) {
	// Even for a tenant that doesn't exist, an unsupported key must win -
	// callers shouldn't learn about cache contents via error ordering.
	svc := newTestService(t)
	svc.Cache.SetSynced()

	_, _, _, err := svc.Query(context.Background(), "does-not-exist", []string{"NOT.a.real.key"}, false)
	if !errors.Is(err, ErrUnsupportedKey) {
		t.Fatalf("got %v, want ErrUnsupportedKey", err)
	}
}

func TestQuery_TenantNotFound(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()

	_, _, _, err := svc.Query(context.Background(), "missing-tenant", []string{"TENANT.id"}, false)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("got %v, want ErrTenantNotFound", err)
	}
}

func TestQuery_TenantIDNamespacePrefixStripped(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&TenantState{TenantID: "t1", Revision: "1"})

	results, _, _, err := svc.Query(context.Background(), "tenant-t1", []string{"TENANT.id"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Value != "t1" {
		t.Fatalf("got %+v, want TENANT.id=t1", results)
	}
}

func TestQuery_StaleCacheEntry(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.cacheEntryTTL = time.Minute
	svc.now = func() time.Time { return time.Unix(1000, 0) }
	svc.Cache.Set(&TenantState{
		TenantID:  "t1",
		Revision:  "1",
		UpdatedAt: time.Unix(1000, 0).Add(-2 * time.Minute),
	})

	_, _, _, err := svc.Query(context.Background(), "t1", []string{"TENANT.id"}, false)
	if !errors.Is(err, ErrCacheEntryStale) {
		t.Fatalf("got %v, want ErrCacheEntryStale", err)
	}
}

func TestQuery_TTLZeroDisablesStaleCheck(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.cacheEntryTTL = 0 // disabled
	svc.Cache.Set(&TenantState{
		TenantID:  "t1",
		Revision:  "1",
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	})

	_, _, _, err := svc.Query(context.Background(), "t1", []string{"TENANT.id"}, false)
	if err != nil {
		t.Fatalf("unexpected error with TTL disabled: %v", err)
	}
}

func TestQuery_FreshEntryWithinTTL(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.cacheEntryTTL = time.Minute
	svc.now = func() time.Time { return time.Unix(1000, 0) }
	svc.Cache.Set(&TenantState{
		TenantID:  "t1",
		Revision:  "1",
		UpdatedAt: time.Unix(1000, 0).Add(-30 * time.Second),
	})

	_, _, _, err := svc.Query(context.Background(), "t1", []string{"TENANT.id"}, false)
	if err != nil {
		t.Fatalf("unexpected error for fresh entry: %v", err)
	}
}

func TestQuery_StrictMissingKeys(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&TenantState{TenantID: "t1", Revision: "1"}) // no RuntimeConfig

	_, _, _, err := svc.Query(context.Background(), "t1", []string{"TENANT.runtime"}, true)
	if !errors.Is(err, ErrStrictMissingKeys) {
		t.Fatalf("got %v, want ErrStrictMissingKeys", err)
	}
}

func TestQuery_NonStrictMissingKeysReturnsListNoError(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&TenantState{TenantID: "t1", Revision: "1"})

	results, missing, _, err := svc.Query(context.Background(), "t1", []string{"TENANT.runtime"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results: got %+v, want empty", results)
	}
	if len(missing) != 1 || missing[0] != "TENANT.runtime" {
		t.Errorf("missing: got %+v, want [TENANT.runtime]", missing)
	}
}

// --- DiscoveryService.Query: success paths per key ---

func fullTenantState() *TenantState {
	return &TenantState{
		TenantID:    "t1",
		DisplayName: "Test Tenant",
		RuntimeConfig: map[string]any{
			"FEATURE_FLAG": "true",
		},
		CloudResources: map[string]any{
			"identityPoolID":    "pool-1",
			"identityPoolARN":   "arn:pool-1",
			"storageBucketName": "bucket-1",
		},
		AuthzPolicy: &AuthzPolicySnapshot{
			IssuerAllowList: []string{"https://issuer.example"},
			SubjectClaim:    "sub",
			ScopesClaim:     "scp",
			Bindings: []AuthzBindingSnapshot{
				{Subject: "svc:authz", Permissions: []string{"config:read"}},
			},
			RoleBindings: []AuthzRoleBindingSnapshot{
				{Role: "admin", Permissions: []string{"config:read", "config:write"}},
			},
		},
		Revision: "42",
	}
}

func TestQuery_AllSupportedKeysPresent(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(fullTenantState())

	keys := make([]string, 0, len(SupportedKeys))
	for k := range SupportedKeys {
		keys = append(keys, k)
	}

	results, missing, revision, err := svc.Query(context.Background(), "t1", keys, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing: got %+v, want none", missing)
	}
	if len(results) != len(keys) {
		t.Errorf("results: got %d, want %d", len(results), len(keys))
	}
	if revision != "42" {
		t.Errorf("revision: got %q, want %q", revision, "42")
	}
}

func TestQuery_IndividualKeyValues(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(fullTenantState())

	tests := []struct {
		key    string
		source string
		check  func(t *testing.T, v any)
	}{
		{"TENANT.id", "tenant.spec.tenantID", func(t *testing.T, v any) {
			if v != "t1" {
				t.Errorf("got %v, want t1", v)
			}
		}},
		{"TENANT.displayName", "tenant.spec.displayName", func(t *testing.T, v any) {
			if v != "Test Tenant" {
				t.Errorf("got %v, want Test Tenant", v)
			}
		}},
		{"TENANT.runtime", "tenant.spec.runtimeConfig", func(t *testing.T, v any) {
			m, ok := v.(map[string]any)
			if !ok || m["FEATURE_FLAG"] != "true" {
				t.Errorf("got %v, want map with FEATURE_FLAG=true", v)
			}
		}},
		{"TENANT.resources.identityPoolID", "tenant.status.cloudResources.identityPoolID", func(t *testing.T, v any) {
			if v != "pool-1" {
				t.Errorf("got %v, want pool-1", v)
			}
		}},
		{"TENANT.resources.identityPoolARN", "tenant.status.cloudResources.identityPoolARN", func(t *testing.T, v any) {
			if v != "arn:pool-1" {
				t.Errorf("got %v, want arn:pool-1", v)
			}
		}},
		{"TENANT.resources.storageBucketName", "tenant.status.cloudResources.storageBucketName", func(t *testing.T, v any) {
			if v != "bucket-1" {
				t.Errorf("got %v, want bucket-1", v)
			}
		}},
		{"AUTHZ.jwt.issuerAllowList", "policy.spec.jwt.issuerAllowList", func(t *testing.T, v any) {
			list, ok := v.([]any)
			if !ok || len(list) != 1 || list[0] != "https://issuer.example" {
				t.Errorf("got %v, want [https://issuer.example]", v)
			}
		}},
		{"AUTHZ.jwt.subjectClaim", "policy.spec.jwt.subjectClaim", func(t *testing.T, v any) {
			if v != "sub" {
				t.Errorf("got %v, want sub", v)
			}
		}},
		{"AUTHZ.jwt.scopesClaim", "policy.spec.jwt.scopesClaim", func(t *testing.T, v any) {
			if v != "scp" {
				t.Errorf("got %v, want scp", v)
			}
		}},
		{"AUTHZ.bindings", "policy.spec.bindings", func(t *testing.T, v any) {
			list, ok := v.([]any)
			if !ok || len(list) != 1 {
				t.Fatalf("got %v, want single-element list", v)
			}
			entry, ok := list[0].(map[string]any)
			if !ok || entry["subject"] != "svc:authz" {
				t.Errorf("got %v, want subject=svc:authz", list[0])
			}
		}},
		{"AUTHZ.roleBindings", "policy.spec.roleBindings", func(t *testing.T, v any) {
			list, ok := v.([]any)
			if !ok || len(list) != 1 {
				t.Fatalf("got %v, want single-element list", v)
			}
			entry, ok := list[0].(map[string]any)
			if !ok || entry["role"] != "admin" {
				t.Errorf("got %v, want role=admin", list[0])
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			results, _, _, err := svc.Query(context.Background(), "t1", []string{tt.key}, true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].Source != tt.source {
				t.Errorf("source: got %q, want %q", results[0].Source, tt.source)
			}
			tt.check(t, results[0].Value)
		})
	}
}

func TestQuery_MissingAuthzKeysWhenPolicyNil(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&TenantState{TenantID: "t1", Revision: "1"}) // AuthzPolicy nil

	authzKeys := []string{
		"AUTHZ.jwt.issuerAllowList",
		"AUTHZ.jwt.subjectClaim",
		"AUTHZ.jwt.scopesClaim",
		"AUTHZ.bindings",
		"AUTHZ.roleBindings",
	}

	results, missing, _, err := svc.Query(context.Background(), "t1", authzKeys, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results: got %+v, want none", results)
	}
	if len(missing) != len(authzKeys) {
		t.Errorf("missing: got %+v, want all authz keys", missing)
	}
}

func TestQuery_MissingCloudResourceFields(t *testing.T) {
	svc := newTestService(t)
	svc.Cache.SetSynced()
	svc.Cache.Set(&TenantState{TenantID: "t1", Revision: "1"}) // CloudResources nil

	resourceKeys := []string{
		"TENANT.resources.identityPoolID",
		"TENANT.resources.identityPoolARN",
		"TENANT.resources.storageBucketName",
	}

	_, missing, _, err := svc.Query(context.Background(), "t1", resourceKeys, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != len(resourceKeys) {
		t.Errorf("missing: got %+v, want all resource keys", missing)
	}
}
