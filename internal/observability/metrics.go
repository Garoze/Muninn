package observability

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds all Prometheus metrics for Muninn.
type Metrics struct {
	// RequestsTotal counts RPC outcomes by operation (query/resolve), result
	// label, and gRPC status code.
	RequestsTotal *prometheus.CounterVec

	// InformerEventsTotal counts config source informer events by type
	// (add/update/delete).
	InformerEventsTotal *prometheus.CounterVec

	// CacheStaleRejectionTotal counts queries rejected due to stale cache entries.
	CacheStaleRejectionTotal prometheus.Counter

	// RequestDuration observes RPC latency in seconds, by operation.
	RequestDuration *prometheus.HistogramVec

	// CacheEntries is the current number of namespaces in the cache.
	CacheEntries prometheus.Gauge

	// CacheSynced is 1 when the informer cache has completed initial sync.
	CacheSynced prometheus.Gauge

	// WebhookRequestsTotal counts /mutate outcomes by outcome label
	// (allowed/denied/error). denied is a correct Allowed:false admission
	// decision (e.g. an unreconcilable SecretProviderClass reference);
	// error is a webhook-side failure (decode/marshal/encode) - kept
	// distinct so one doesn't hide the other in this metric.
	WebhookRequestsTotal *prometheus.CounterVec

	// WebhookRequestDuration observes /mutate request latency in seconds,
	// by outcome label.
	WebhookRequestDuration *prometheus.HistogramVec

	// WebhookInjectionsTotal counts /mutate requests that actually injected
	// the volume/init-container/sidecar into a Pod (as opposed to requests
	// for unannotated Pods, or already-injected ones).
	WebhookInjectionsTotal prometheus.Counter
}

// NewMetrics registers and returns all Muninn metrics on the given registerer.
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
			Help:      "Config source informer event volume.",
		}, []string{"event"}),

		CacheStaleRejectionTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "discovery",
			Name:      "cache_stale_reject_total",
			Help:      "Total query rejections due to stale cache entries.",
		}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "discovery",
			Name:      "request_duration_seconds",
			Help:      "Discovery RPC latency in seconds.",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		}, []string{"operation"}),

		CacheEntries: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "discovery",
			Name:      "cache_entries",
			Help:      "Number of namespaces currently cached.",
		}),

		CacheSynced: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "discovery",
			Name:      "cache_synced",
			Help:      "1 if the informer cache is synced and ready to serve.",
		}),

		WebhookRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "webhook",
			Name:      "requests_total",
			Help:      "Total /mutate admission review outcomes.",
		}, []string{"outcome"}),

		WebhookRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "webhook",
			Name:      "request_duration_seconds",
			Help:      "/mutate request latency in seconds.",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		}, []string{"outcome"}),

		WebhookInjectionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "webhook",
			Name:      "injections_total",
			Help:      "Total /mutate requests that actually injected the config volume/containers.",
		}),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.InformerEventsTotal,
		m.CacheStaleRejectionTotal,
		m.RequestDuration,
		m.CacheEntries,
		m.CacheSynced,
		m.WebhookRequestsTotal,
		m.WebhookRequestDuration,
		m.WebhookInjectionsTotal,
	)

	return m
}
