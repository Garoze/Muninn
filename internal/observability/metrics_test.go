package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetrics_RegistersWithoutPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	if m.RequestsTotal == nil || m.InformerEventsTotal == nil || m.CacheStaleRejectionTotal == nil ||
		m.QueryDuration == nil || m.CacheEntries == nil || m.CacheSynced == nil ||
		m.WebhookRequestsTotal == nil || m.WebhookRequestDuration == nil || m.WebhookInjectionsTotal == nil {
		t.Fatal("NewMetrics left a nil field")
	}
}

// TestRequestsTotal_LabelCardinality guards against the label mismatch that
// previously panicked on every Query call: the metric was once declared with
// a single "event" label but the transport handler always calls
// WithLabelValues(operation, result, code) - three values.
func TestRequestsTotal_LabelCardinality(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WithLabelValues(operation, result, code) panicked: %v", r)
		}
	}()

	m.RequestsTotal.WithLabelValues("query", "success", "OK").Inc()

	if got := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("query", "success", "OK")); got != 1 {
		t.Errorf("got %v, want 1", got)
	}
}

// TestRequestsTotal_OperationsAreDistinctSeries confirms Query and Resolve
// outcomes are counted separately (by the "operation" label) rather than
// merged into the same series.
func TestRequestsTotal_OperationsAreDistinctSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.RequestsTotal.WithLabelValues("query", "success", "OK").Inc()
	m.RequestsTotal.WithLabelValues("resolve", "success", "OK").Inc()
	m.RequestsTotal.WithLabelValues("resolve", "success", "OK").Inc()

	if got := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("query", "success", "OK")); got != 1 {
		t.Errorf("query: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("resolve", "success", "OK")); got != 2 {
		t.Errorf("resolve: got %v, want 2", got)
	}
}

func TestRequestsTotal_WrongLabelCountPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic calling WithLabelValues with wrong arity, got none")
		}
	}()

	m.RequestsTotal.WithLabelValues("query", "only-two-labels")
}

func TestNewMetrics_DoubleRegisterPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	NewMetrics(reg)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic registering metrics twice on the same registry, got none")
		}
	}()

	NewMetrics(reg)
}

func TestInformerEventsTotal_SingleLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.InformerEventsTotal.WithLabelValues("add").Inc()
	m.InformerEventsTotal.WithLabelValues("update").Inc()
	m.InformerEventsTotal.WithLabelValues("delete").Inc()

	if got := testutil.ToFloat64(m.InformerEventsTotal.WithLabelValues("add")); got != 1 {
		t.Errorf("add: got %v, want 1", got)
	}
}

func TestCacheEntries_Gauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.CacheEntries.Set(5)
	if got := testutil.ToFloat64(m.CacheEntries); got != 5 {
		t.Errorf("got %v, want 5", got)
	}
}

// TestWebhookRequestsTotal_OutcomesAreDistinctSeries confirms allowed and
// error outcomes are counted separately (by the "outcome" label).
func TestWebhookRequestsTotal_OutcomesAreDistinctSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.WebhookRequestsTotal.WithLabelValues("allowed").Inc()
	m.WebhookRequestsTotal.WithLabelValues("error").Inc()
	m.WebhookRequestsTotal.WithLabelValues("error").Inc()

	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("allowed")); got != 1 {
		t.Errorf("allowed: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("error")); got != 2 {
		t.Errorf("error: got %v, want 2", got)
	}
}

func TestWebhookInjectionsTotal_Counter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.WebhookInjectionsTotal.Inc()
	m.WebhookInjectionsTotal.Inc()

	if got := testutil.ToFloat64(m.WebhookInjectionsTotal); got != 2 {
		t.Errorf("got %v, want 2", got)
	}
}

func TestCacheSynced_Gauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	if got := testutil.ToFloat64(m.CacheSynced); got != 0 {
		t.Errorf("initial: got %v, want 0", got)
	}
	m.CacheSynced.Set(1)
	if got := testutil.ToFloat64(m.CacheSynced); got != 1 {
		t.Errorf("after Set(1): got %v, want 1", got)
	}
}
