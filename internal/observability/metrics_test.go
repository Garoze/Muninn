package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetrics_RegistersWithoutPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	if m.QueriesTotal == nil || m.InformerEventsTotal == nil || m.CacheStaleRejectionTotal == nil ||
		m.QueryDuration == nil || m.CacheEntries == nil || m.CacheSynced == nil {
		t.Fatal("NewMetrics left a nil field")
	}
}

// TestQueriesTotal_LabelCardinality guards against the label mismatch that
// previously panicked on every Query call: QueriesTotal was declared with a
// single "event" label but the transport handler always calls
// WithLabelValues(result, code) - two values.
func TestQueriesTotal_LabelCardinality(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WithLabelValues(result, code) panicked: %v", r)
		}
	}()

	m.QueriesTotal.WithLabelValues("success", "OK").Inc()

	if got := testutil.ToFloat64(m.QueriesTotal.WithLabelValues("success", "OK")); got != 1 {
		t.Errorf("got %v, want 1", got)
	}
}

func TestQueriesTotal_WrongLabelCountPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic calling WithLabelValues with wrong arity, got none")
		}
	}()

	m.QueriesTotal.WithLabelValues("only-one-label")
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
