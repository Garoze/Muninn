package observability

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds all Prometheus metris for Muninn.
type Metrics struct {
	// RequestsTotal counts RPC outcomes by operation (query/resolve), result
	// label, and gRPC status code.
	RequestsTotal *prometheus.CounterVec

	// InformerEventsTotal count CRD informer events by type (add/update/delete)
	InformerEventsTotal *prometheus.CounterVec

	// CacheStaleRejectionTotal counts queries rejected due to stale cache entries.
	CacheStaleRejectionTotal prometheus.Counter

	// QueryDuration observes query latency in seconds.
	QueryDuration *prometheus.HistogramVec

	// CacheEntries is the current number of tenants in the cache.
	CacheEntries prometheus.Gauge

	// CacheSynced is 1 when the informer cache has completed initial sync.
	CacheSynced prometheus.Gauge
}

// NewMetrics registers and returns all Muninn metrics on the giver registerer.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "discovery",
			Name:      "requests_total",
			Help:      "Total discovery RPC outcomes.",
		}, []string{"operation", "result", "code"}),

		InformerEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "discovery",
			Name:      "informer_events_total",
			Help:      "CRD informer event volume.",
		}, []string{"event"}),

		CacheStaleRejectionTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "discovery",
			Name:      "cache_stale_reject_total",
			Help:      "Total query rejections due to stale cache entries.",
		}),

		QueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "discovery",
			Name:      "query_duration_seconds",
			Help:      "Discovery query latency in seconds.",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		}, []string{"operation"}),

		CacheEntries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "discovery",
			Name:      "cache_entries",
			Help:      "Number of tenants currently cached.",
		}),

		CacheSynced: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "discovery",
			Name:      "cache_synced",
			Help:      "1 if informer cache is sunced and ready to server.",
		}),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.InformerEventsTotal,
		m.CacheStaleRejectionTotal,
		m.QueryDuration,
		m.CacheEntries,
		m.CacheSynced,
	)

	return m
}
