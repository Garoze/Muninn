package grpc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

func newTestHandler(t *testing.T) *DiscoveryHandler {
	t.Helper()
	log := zap.NewNop()
	return &DiscoveryHandler{
		Service: app.NewDiscoveryService(&config.Config{}, log),
		Metrics: observability.NewMetrics(prometheus.NewRegistry()),
		Logger:  log,
	}
}

// --- Query ---

func TestQuery_CacheNotSynced(t *testing.T) {
	h := newTestHandler(t)

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.id"},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("got %v, want Unavailable", err)
	}
}

func TestQuery_Success(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{TenantID: "t1", DisplayName: "Test", Revision: "1"})

	resp, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.id", "TENANT.displayName"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Values) != 2 {
		t.Fatalf("got %d values, want 2", len(resp.Values))
	}
	if resp.Revision != "1" {
		t.Errorf("revision: got %q, want %q", resp.Revision, "1")
	}
}

func TestQuery_MissingKeysNonStrict(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{TenantID: "t1", Revision: "1"})

	resp, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.runtime"},
		Strict:   false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MissingKeys) != 1 || resp.MissingKeys[0] != "TENANT.runtime" {
		t.Errorf("missing_keys: got %v, want [TENANT.runtime]", resp.MissingKeys)
	}
}

func TestQuery_StrictMissingMapsToInvalidArgument(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{TenantID: "t1", Revision: "1"})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"AUTHZ.jwt.subjectClaim"},
		Strict:   true,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
}

func TestQuery_TenantNotFoundMapsToNotFound(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "missing",
		Keys:     []string{"TENANT.id"},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
}

func TestQuery_UnsupportedKeyMapsToInvalidArgument(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{TenantID: "t1", Revision: "1"})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"NOT.a.key"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
}

func TestQuery_EmptyTenantIDMapsToInvalidArgument(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Keys: []string{"TENANT.id"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
}

func TestQuery_UnserializableValueMapsToInternal(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{
		TenantID: "t1",
		Revision: "1",
		// structpb.NewValue cannot represent a channel; this forces the
		// handler's serialization-failure branch.
		RuntimeConfig: map[string]any{"bad": make(chan int)},
	})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.runtime"},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("got %v, want Internal", err)
	}
}

func TestQuery_AuthzBindingsSerialization(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{
		TenantID: "t1",
		Revision: "1",
		AuthzPolicy: &app.AuthzPolicySnapshot{
			Bindings: []app.AuthzBindingSnapshot{
				{Subject: "admin@example.com", Permissions: []string{"read", "write"}},
			},
		},
	})

	resp, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"AUTHZ.bindings"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Values) != 1 || resp.Values[0].GetValue().GetListValue() == nil {
		t.Fatalf("expected list value for AUTHZ.bindings, got %+v", resp.Values)
	}
}

func TestQuery_MetricsRecordedOnSuccessAndFailure(t *testing.T) {
	h := newTestHandler(t)

	// Failure: cache not synced -> unavailable/Unavailable.
	_, _ = h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.id"},
	})
	if got := testutil.ToFloat64(h.Metrics.QueriesTotal.WithLabelValues("unavailable", "Unavailable")); got != 1 {
		t.Fatalf("unavailable metric: got %v, want 1", got)
	}

	// Success.
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{TenantID: "t1", Revision: "1"})
	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.id"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := testutil.ToFloat64(h.Metrics.QueriesTotal.WithLabelValues("success", "OK")); got != 1 {
		t.Fatalf("success metric: got %v, want 1", got)
	}
}

func TestQuery_StaleCacheIncrementsStaleMetric(t *testing.T) {
	h := newTestHandler(t)
	h.Service = app.NewDiscoveryService(&config.Config{CacheEntryTTL: time.Millisecond}, zap.NewNop())
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.TenantState{
		TenantID:  "t1",
		Revision:  "1",
		UpdatedAt: time.Now().Add(-time.Hour),
	})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		TenantId: "t1",
		Keys:     []string{"TENANT.id"},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("got %v, want Unavailable", err)
	}
	if got := testutil.ToFloat64(h.Metrics.CacheStaleRejectionTotal); got != 1 {
		t.Fatalf("got %v, want 1", got)
	}
}

// --- Describe ---

func TestDescribe(t *testing.T) {
	h := newTestHandler(t)

	resp, err := h.Describe(context.Background(), &discoveryv1.DescribeRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.SupportedKeys) != len(app.SupportedKeys) {
		t.Errorf("got %d keys, want %d", len(resp.SupportedKeys), len(app.SupportedKeys))
	}

	found := false
	for _, k := range resp.SupportedKeys {
		if k.GetKey() == "TENANT.id" {
			found = true
			if k.GetTypeHint() != "string" {
				t.Errorf("TENANT.id type_hint: got %q, want %q", k.GetTypeHint(), "string")
			}
		}
	}
	if !found {
		t.Error("expected TENANT.id in supported keys")
	}
}

func TestDescribe_SupportedKeysAreSorted(t *testing.T) {
	h := newTestHandler(t)

	resp, err := h.Describe(context.Background(), &discoveryv1.DescribeRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	keys := make([]string, 0, len(resp.SupportedKeys))
	for _, k := range resp.SupportedKeys {
		keys = append(keys, k.GetKey())
	}

	want := append([]string(nil), keys...)
	sort.Strings(want)
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("not sorted\n got: %v\nwant: %v", keys, want)
	}
}

func TestDescribe_MissingDescriptionUsesFallback(t *testing.T) {
	const tempKey = "TENANT.test.missingDescription"
	app.SupportedKeys[tempKey] = "string"
	t.Cleanup(func() { delete(app.SupportedKeys, tempKey) })

	h := newTestHandler(t)

	resp, err := h.Describe(context.Background(), &discoveryv1.DescribeRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, k := range resp.SupportedKeys {
		if k.GetKey() == tempKey {
			if k.GetDescription() != "no description provided" {
				t.Errorf("got %q, want fallback description", k.GetDescription())
			}
			return
		}
	}
	t.Fatalf("expected temp key %q in response", tempKey)
}

// --- classifyError ---

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantResult string
		wantCode   string
		wantGRPC   codes.Code
	}{
		{"tenant not found", app.ErrTenantNotFound, "not_found", "NotFound", codes.NotFound},
		{"unsupported key", app.ErrUnsupportedKey, "invalid_argument", "InvalidArgument", codes.InvalidArgument},
		{"tenant id required", app.ErrTenantIDRequired, "invalid_argument", "InvalidArgument", codes.InvalidArgument},
		{"strict missing keys", app.ErrStrictMissingKeys, "invalid_argument", "InvalidArgument", codes.InvalidArgument},
		{"cache not synced", app.ErrCacheNotSynced, "unavailable", "Unavailable", codes.Unavailable},
		{"stale cache entry", app.ErrCacheEntryStale, "unavailable", "Unavailable", codes.Unavailable},
		{"unknown error", errors.New("boom"), "internal", "Internal", codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.err)
			if got.resultLabel != tt.wantResult || got.codelabel != tt.wantCode || got.grpcCode != tt.wantGRPC {
				t.Errorf("got (%s, %s, %s), want (%s, %s, %s)",
					got.resultLabel, got.codelabel, got.grpcCode,
					tt.wantResult, tt.wantCode, tt.wantGRPC)
			}
		})
	}
}

func TestClassifyError_WrappedSentinel(t *testing.T) {
	wrapped := fmt.Errorf("query failed: %w", app.ErrTenantNotFound)
	got := classifyError(wrapped)
	if got.resultLabel != "not_found" || got.grpcCode != codes.NotFound {
		t.Errorf("wrapped sentinel not classified: got %+v", got)
	}
}

func TestClassifyError_NilError(t *testing.T) {
	// classifyError has no nil-guard; errors.Is(nil, target) is well-defined
	// (always false), so nil should fall through to the default Internal case
	// rather than panic.
	got := classifyError(nil)
	if got.grpcCode != codes.Internal {
		t.Errorf("got %v, want Internal for nil error", got.grpcCode)
	}
}
