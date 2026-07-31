package kube

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/cache"

	v1alpha1 "github.com/garoze/muninn/api/v1alpha1"
	"github.com/garoze/muninn/internal/app"
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

// --- applyPatch: the patch-based cache merge ---

func TestApplyPatch_EmptyTenantIDIsNoop(t *testing.T) {
	w := newTestWatcher(t)
	w.applyPatch(tenantPatch{tenantID: ""})

	if w.appCache.Len() != 0 {
		t.Errorf("got %d cached tenants, want 0", w.appCache.Len())
	}
}

func TestApplyPatch_CreatesNewTenant(t *testing.T) {
	w := newTestWatcher(t)
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})

	state := w.appCache.Get("t1")
	if state == nil {
		t.Fatal("expected tenant to be cached")
	}
	if state.DisplayName != "Arasaka" {
		t.Errorf("DisplayName: got %q, want %q", state.DisplayName, "Arasaka")
	}
}

// TestApplyPatch_ResourceScopedMerge is the core claim this project makes:
// a Policy update must not clobber TenantConfig data, and vice versa,
// because each CRD's handler only sets the tenantPatch fields it owns.
func TestApplyPatch_ResourceScopedMerge(t *testing.T) {
	w := newTestWatcher(t)
	name := "Arasaka"

	// Tenant patch sets displayName only.
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})

	// TenantConfig patch sets runtimeConfig only.
	w.applyPatch(tenantPatch{
		tenantID:      "t1",
		runtimeConfig: map[string]any{"LOG_LEVEL": "info"},
		revision:      "2",
	})

	// Policy patch sets authzPolicy only.
	w.applyPatch(tenantPatch{
		tenantID:    "t1",
		authzPolicy: &app.AuthzPolicySnapshot{SubjectClaim: "sub"},
		revision:    "3",
	})

	state := w.appCache.Get("t1")
	if state == nil {
		t.Fatal("expected tenant to be cached")
	}
	if state.DisplayName != "Arasaka" {
		t.Errorf("DisplayName was clobbered: got %q, want %q", state.DisplayName, "Arasaka")
	}
	if state.RuntimeConfig["LOG_LEVEL"] != "info" {
		t.Errorf("RuntimeConfig was clobbered: got %+v", state.RuntimeConfig)
	}
	if state.AuthzPolicy == nil || state.AuthzPolicy.SubjectClaim != "sub" {
		t.Errorf("AuthzPolicy was clobbered: got %+v", state.AuthzPolicy)
	}
	if state.Revision != "3" {
		t.Errorf("Revision: got %q, want latest patch's revision %q", state.Revision, "3")
	}
}

func TestApplyPatch_ClearDisplayNameOnlyClearsThatField(t *testing.T) {
	w := newTestWatcher(t)
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})
	w.applyPatch(tenantPatch{
		tenantID:      "t1",
		runtimeConfig: map[string]any{"LOG_LEVEL": "info"},
		revision:      "2",
	})

	w.applyPatch(tenantPatch{tenantID: "t1", clearDisplayName: true, revision: "3"})

	state := w.appCache.Get("t1")
	if state == nil {
		t.Fatal("expected tenant to remain cached (RuntimeConfig still present)")
	}
	if state.DisplayName != "" {
		t.Errorf("DisplayName: got %q, want cleared", state.DisplayName)
	}
	if state.RuntimeConfig["LOG_LEVEL"] != "info" {
		t.Errorf("RuntimeConfig should be untouched by a displayName clear: got %+v", state.RuntimeConfig)
	}
}

func TestApplyPatch_ClearRuntimeConfigOnlyClearsThatField(t *testing.T) {
	w := newTestWatcher(t)
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})
	w.applyPatch(tenantPatch{
		tenantID:      "t1",
		runtimeConfig: map[string]any{"LOG_LEVEL": "info"},
		revision:      "2",
	})

	w.applyPatch(tenantPatch{tenantID: "t1", clearRuntimeConfig: true, revision: "3"})

	state := w.appCache.Get("t1")
	if state == nil {
		t.Fatal("expected tenant to remain cached (DisplayName still present)")
	}
	if state.RuntimeConfig != nil {
		t.Errorf("RuntimeConfig: got %+v, want cleared", state.RuntimeConfig)
	}
	if state.DisplayName != "Arasaka" {
		t.Errorf("DisplayName should be untouched by a runtimeConfig clear: got %q", state.DisplayName)
	}
}

func TestApplyPatch_DeletesEntryWhenAllFieldsEmpty(t *testing.T) {
	w := newTestWatcher(t)
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})

	if w.appCache.Get("t1") == nil {
		t.Fatal("precondition: expected tenant to be cached")
	}

	w.applyPatch(tenantPatch{tenantID: "t1", clearDisplayName: true, revision: "2"})

	if w.appCache.Get("t1") != nil {
		t.Error("expected tenant to be removed once all fields are empty")
	}
}

func TestApplyPatch_RevisionOnlyUpdatesWhenNonEmpty(t *testing.T) {
	w := newTestWatcher(t)
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: ""})

	state := w.appCache.Get("t1")
	if state.Revision != "1" {
		t.Errorf("Revision: got %q, want retained %q", state.Revision, "1")
	}
}

func TestApplyPatch_UpdatedAtDefaultsToNowWhenZero(t *testing.T) {
	w := newTestWatcher(t)
	before := time.Now().UTC()
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1"})
	after := time.Now().UTC()

	state := w.appCache.Get("t1")
	if state.UpdatedAt.Before(before) || state.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not within [%v, %v]", state.UpdatedAt, before, after)
	}
}

func TestApplyPatch_UpdatedAtUsesExplicitValue(t *testing.T) {
	w := newTestWatcher(t)
	explicit := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	name := "Arasaka"
	w.applyPatch(tenantPatch{tenantID: "t1", displayName: &name, revision: "1", updated: explicit})

	state := w.appCache.Get("t1")
	if !state.UpdatedAt.Equal(explicit) {
		t.Errorf("UpdatedAt: got %v, want %v", state.UpdatedAt, explicit)
	}
}

// --- tenantCacheKey ---

func TestTenantCacheKey(t *testing.T) {
	tests := []struct {
		name string
		t    *v1alpha1.Tenant
		want string
	}{
		{"nil tenant", nil, ""},
		{"tenantID set", &v1alpha1.Tenant{Spec: v1alpha1.TenantSpec{TenantID: "t1"}}, "t1"},
		{
			"falls back to status.namespace",
			&v1alpha1.Tenant{Status: v1alpha1.TenantStatus{Namespace: "tenant-t2"}},
			"t2",
		},
		{
			"tenantID takes precedence over namespace",
			&v1alpha1.Tenant{
				Spec:   v1alpha1.TenantSpec{TenantID: "t1"},
				Status: v1alpha1.TenantStatus{Namespace: "tenant-t2"},
			},
			"t1",
		},
		{"both empty", &v1alpha1.Tenant{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantCacheKey(tt.t); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- tenantIDFromNamespace ---

func TestTenantIDFromNamespace(t *testing.T) {
	tests := []struct {
		namespace string
		want      string
	}{
		{"tenant-arasaka", "arasaka"},
		{"", ""},
		{"arasaka", "arasaka"}, // no "tenant-" prefix: returned unchanged
	}

	for _, tt := range tests {
		t.Run(tt.namespace, func(t *testing.T) {
			if got := tenantIDFromNamespace(tt.namespace); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- toAnyMap ---

func TestToAnyMap(t *testing.T) {
	if got := toAnyMap(nil); got != nil {
		t.Errorf("nil input: got %+v, want nil", got)
	}

	got := toAnyMap(map[string]string{"a": "1", "b": "2"})
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %+v", got)
	}
}

// --- toCloudResourcesMap ---

func TestToCloudResourcesMap(t *testing.T) {
	t.Run("all empty returns nil", func(t *testing.T) {
		if got := toCloudResourcesMap(v1alpha1.CloudResources{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("only whitelisted fields are mapped", func(t *testing.T) {
		got := toCloudResourcesMap(v1alpha1.CloudResources{
			IdentityPoolID:    "pool-1",
			IdentityPoolARN:   "arn:pool-1",
			IdentityClientID:  "client-1", // not in SupportedKeys - must be dropped
			IdentityDomain:    "arasaka.example",
			StorageBucketName: "bucket-1",
			StorageBucketARN:  "arn:bucket-1", // not in SupportedKeys - must be dropped
		})

		want := map[string]any{
			"identityPoolID":    "pool-1",
			"identityPoolARN":   "arn:pool-1",
			"storageBucketName": "bucket-1",
		}
		if len(got) != len(want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: got %v, want %v", k, got[k], v)
			}
		}
	})

	t.Run("partial fields only include set ones", func(t *testing.T) {
		got := toCloudResourcesMap(v1alpha1.CloudResources{IdentityPoolID: "pool-1"})
		if len(got) != 1 || got["identityPoolID"] != "pool-1" {
			t.Errorf("got %+v", got)
		}
	})
}

// --- toAuthzPolicySnapshot ---

func TestToAuthzPolicySnapshot_Nil(t *testing.T) {
	if got := toAuthzPolicySnapshot(nil); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestToAuthzPolicySnapshot_CopiesFields(t *testing.T) {
	pol := &v1alpha1.Policy{
		Spec: v1alpha1.PolicySpec{
			JWT: v1alpha1.JWTConfig{
				IssuerAllowList: []string{"https://issuer.example"},
				SubjectClaim:    "sub",
				ScopesClaim:     "scp",
			},
			Bindings: []v1alpha1.Binding{
				{Subject: "svc:authz", Permissions: []string{"config:read"}},
			},
			RoleBindings: []v1alpha1.RoleBinding{
				{Role: "admin", Permissions: []string{"config:read", "config:write"}},
			},
		},
	}

	got := toAuthzPolicySnapshot(pol)
	if got == nil {
		t.Fatal("got nil")
	}
	if len(got.IssuerAllowList) != 1 || got.IssuerAllowList[0] != "https://issuer.example" {
		t.Errorf("IssuerAllowList: got %+v", got.IssuerAllowList)
	}
	if got.SubjectClaim != "sub" || got.ScopesClaim != "scp" {
		t.Errorf("claims: got subject=%q scopes=%q", got.SubjectClaim, got.ScopesClaim)
	}
	if len(got.Bindings) != 1 || got.Bindings[0].Subject != "svc:authz" {
		t.Errorf("Bindings: got %+v", got.Bindings)
	}
	if len(got.RoleBindings) != 1 || got.RoleBindings[0].Role != "admin" {
		t.Errorf("RoleBindings: got %+v", got.RoleBindings)
	}
}

func TestToAuthzPolicySnapshot_DeepCopiesSlices(t *testing.T) {
	pol := &v1alpha1.Policy{
		Spec: v1alpha1.PolicySpec{
			JWT: v1alpha1.JWTConfig{IssuerAllowList: []string{"https://issuer.example"}},
			Bindings: []v1alpha1.Binding{
				{Subject: "svc:authz", Permissions: []string{"config:read"}},
			},
		},
	}

	got := toAuthzPolicySnapshot(pol)

	// Mutate the source after snapshotting; the snapshot must be unaffected.
	pol.Spec.JWT.IssuerAllowList[0] = "mutated"
	pol.Spec.Bindings[0].Permissions[0] = "mutated"

	if got.IssuerAllowList[0] != "https://issuer.example" {
		t.Errorf("IssuerAllowList shares backing array with source: got %q", got.IssuerAllowList[0])
	}
	if got.Bindings[0].Permissions[0] != "config:read" {
		t.Errorf("Bindings[0].Permissions shares backing array with source: got %q", got.Bindings[0].Permissions[0])
	}
}

// --- extract*: direct objects, tombstones, and wrong types ---

func TestExtractTenant(t *testing.T) {
	want := &v1alpha1.Tenant{Spec: v1alpha1.TenantSpec{TenantID: "t1"}}

	t.Run("direct object", func(t *testing.T) {
		if got := extractTenant(want); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("tombstone-wrapped object", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: want}
		if got := extractTenant(tomb); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("wrong type returns nil", func(t *testing.T) {
		if got := extractTenant(&v1alpha1.Policy{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("tombstone wrapping wrong type returns nil", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: &v1alpha1.Policy{}}
		if got := extractTenant(tomb); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

func TestExtractTenantConfig(t *testing.T) {
	want := &v1alpha1.TenantConfig{Spec: v1alpha1.TenantConfigSpec{RuntimeConfig: map[string]string{"a": "1"}}}

	t.Run("direct object", func(t *testing.T) {
		if got := extractTenantConfig(want); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("tombstone-wrapped object", func(t *testing.T) {
		// Regression guard: extractTenantConfig previously never unwrapped
		// tombstones, silently dropping TenantConfig deletes delivered via
		// cache.DeletedFinalStateUnknown.
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: want}
		if got := extractTenantConfig(tomb); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("wrong type returns nil", func(t *testing.T) {
		if got := extractTenantConfig(&v1alpha1.Tenant{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("tombstone wrapping wrong type returns nil", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: &v1alpha1.Tenant{}}
		if got := extractTenantConfig(tomb); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

func TestExtractPolicy(t *testing.T) {
	want := &v1alpha1.Policy{Spec: v1alpha1.PolicySpec{JWT: v1alpha1.JWTConfig{SubjectClaim: "sub"}}}

	t.Run("direct object", func(t *testing.T) {
		if got := extractPolicy(want); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("tombstone-wrapped object", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: want}
		if got := extractPolicy(tomb); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("wrong type returns nil", func(t *testing.T) {
		if got := extractPolicy(&v1alpha1.Tenant{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("tombstone wrapping wrong type returns nil", func(t *testing.T) {
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: &v1alpha1.Tenant{}}
		if got := extractPolicy(tomb); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

// --- unwrapTombstone ---

func TestUnwrapTombstone(t *testing.T) {
	t.Run("value tombstone", func(t *testing.T) {
		inner := &v1alpha1.Tenant{}
		tomb := cache.DeletedFinalStateUnknown{Key: "t1", Obj: inner}
		if got := unwrapTombstone(tomb); got != inner {
			t.Errorf("got %+v, want %+v", got, inner)
		}
	})

	t.Run("pointer tombstone", func(t *testing.T) {
		inner := &v1alpha1.Tenant{}
		tomb := &cache.DeletedFinalStateUnknown{Key: "t1", Obj: inner}
		if got := unwrapTombstone(tomb); got != inner {
			t.Errorf("got %+v, want %+v", got, inner)
		}
	})

	t.Run("non-tombstone returns nil", func(t *testing.T) {
		if got := unwrapTombstone(&v1alpha1.Tenant{}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}
