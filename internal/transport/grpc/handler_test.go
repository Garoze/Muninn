package grpc

import (
	"context"
	"errors"
	"fmt"
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
		Service: app.NewDiscoveryService(&config.Config{ConfigMapLabelSelector: "muninn.io/config=runtime"}, log),
		Metrics: observability.NewMetrics(prometheus.NewRegistry()),
		Logger:  log,
	}
}

// --- Query ---

func TestQuery_CacheNotSynced(t *testing.T) {
	h := newTestHandler(t)

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL"},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("got %v, want Unavailable", err)
	}
}

func TestQuery_Success(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm1": {"LOG_LEVEL": "info", "FEATURE_FLAG": "true"}},
		Revision:  "1",
	})

	resp, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL", "FEATURE_FLAG"},
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
	h.Service.Cache.Set(&app.ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {}}, Revision: "1"})

	resp, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL"},
		Strict:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MissingKeys) != 1 || resp.MissingKeys[0] != "LOG_LEVEL" {
		t.Errorf("missing_keys: got %v, want [LOG_LEVEL]", resp.MissingKeys)
	}
}

func TestQuery_StrictMissingMapsToInvalidArgument(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {}}, Revision: "1"})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL"},
		Strict:    true,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
}

func TestQuery_NamespaceNotFoundMapsToNotFound(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "missing",
		Keys:      []string{"LOG_LEVEL"},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
}

func TestQuery_EmptyNamespaceMapsToInvalidArgument(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Keys: []string{"LOG_LEVEL"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
}

func TestQuery_UnserializableValueMapsToInternal(t *testing.T) {
	h := newTestHandler(t)
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.ConfigEntry{
		Namespace: "ns1",
		Revision:  "1",
		// structpb.NewValue cannot represent a channel; this forces the
		// handler's serialization-failure branch.
		Sources: map[string]map[string]any{"cm1": {"bad": make(chan int)}},
	})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"bad"},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("got %v, want Internal", err)
	}
}

func TestQuery_MetricsRecordedOnSuccessAndFailure(t *testing.T) {
	h := newTestHandler(t)

	// Failure: cache not synced -> unavailable/Unavailable.
	_, _ = h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL"},
	})
	if got := testutil.ToFloat64(h.Metrics.QueriesTotal.WithLabelValues("unavailable", "Unavailable")); got != 1 {
		t.Fatalf("unavailable metric: got %v, want 1", got)
	}

	// Success.
	h.Service.Cache.SetSynced()
	h.Service.Cache.Set(&app.ConfigEntry{Namespace: "ns1", Sources: map[string]map[string]any{"cm1": {"LOG_LEVEL": "info"}}, Revision: "1"})
	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL"},
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
	h.Service.Cache.Set(&app.ConfigEntry{
		Namespace: "ns1",
		Sources:   map[string]map[string]any{"cm1": {"LOG_LEVEL": "info"}},
		Revision:  "1",
		UpdatedAt: time.Now().Add(-time.Hour),
	})

	_, err := h.Query(context.Background(), &discoveryv1.QueryRequest{
		Namespace: "ns1",
		Keys:      []string{"LOG_LEVEL"},
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
	if len(resp.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(resp.Sources))
	}
	if resp.Sources[0].GetKind() != "ConfigMap" {
		t.Errorf("kind: got %q, want ConfigMap", resp.Sources[0].GetKind())
	}
	if resp.Sources[0].GetLabelSelector() != "muninn.io/config=runtime" {
		t.Errorf("label_selector: got %q, want muninn.io/config=runtime", resp.Sources[0].GetLabelSelector())
	}
	if resp.Sources[0].GetScope() != "namespace" {
		t.Errorf("scope: got %q, want namespace", resp.Sources[0].GetScope())
	}
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
		{"namespace not found", app.ErrNamespaceNotFound, "not_found", "NotFound", codes.NotFound},
		{"namespace required", app.ErrNamespaceRequired, "invalid_argument", "InvalidArgument", codes.InvalidArgument},
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
	wrapped := fmt.Errorf("query failed: %w", app.ErrNamespaceNotFound)
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
