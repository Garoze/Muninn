package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds process-level configuration for Muninn.
// All fields are populated from environment variables with sensible defaults.
type Config struct {
	// GrpcServiceAddr is the bind address for the Muninn gRPC service.
	GrpcServiceAddr string

	// GrpcProbeAddr is the bind address for the gRPC health probe server.
	GrpcProbeAddr string

	// MetricsAddr is the bind address for the Prometheus metrics HTTP server.
	MetricsAddr string

	// OTELExporterEndpoint is the host:port (no scheme) for the OTLP trace exporter.
	OTELExporterEndpoint string

	// TraceSampleRatio is the probability (0.0-1.0) that the new root span is sampled.
	// Uses ParentBased wrapping so inbound sampled traces are alwys honored.
	TraceSampleRatio float64

	// KubeConfigPath is an optional path to a kubeconfig file.
	// When empty, in-cluster config is used.
	KubeConfigPath string

	// ConfigMapLabelSelector scopes which ConfigMaps the watcher watches.
	ConfigMapLabelSelector string

	// CacheEntryTTL controls staleness enforcement for tenant cache entries.
	// Zero disables stale-entry rejection.
	CacheEntryTTL time.Duration

	// StartupTimeout is how long the Fx application is given to complete its
	// OnStart hooks, including informer cache sync. Defaults to 2 minutes.
	StartupTimeout time.Duration

	// WebhookAddr is the bind address for the mutating webhook HTTPS server.
	WebhookAddr string

	// WebhookTLSCertPath is the path to the TLS certificate served by the
	// webhook HTTPS server (populated by cert-manager in-cluster)
	WebhookTLSCertPath string

	// WebhookTLSKeyPath is the path to the TLS private key paired with
	// WebhookTLSCertPath.
	WebhookTLSKeyPath string

	// SelfAddr is the in-cluster address consumers of Muninn's own gRPC API
	// should dial - specifically, what the webhook stamps into the --addr
	// flag of the init container/sidecar it injects into consumer Pods, since
	// those run outside Muninn's own Pod and need a stable address rather than
	// a Pod IP or port-forward.
	SelfAddr string

	// InjectImage is the container image used for the init container and
	// sidecar the webhook injects into opted-in Pods. Must match the image
	// this webhook's own Deployment runs (both invoke the same muninn binary,
	// just via `resolve` instead of `webhook`). Not derived automatically -
	// Kubernetes' Downward API has no fieldRef for a container's own image -
	// so this is set explicitly in config/manager/deployment.yaml, via a YAML
	// anchor on that Deployment's own image field rather than a hand-copied
	// value, so the two can't drift out of sync.
	InjectImage string

	// EnabledConfigSources restricts which registered ConfigSource kinds
	// (matched against each source's Kind()) actually run, by name.
	// Empty (the default) means every registered source is enabled - this
	// only narrows the set already registered in code, it can't enable a
	// source that isn't registered.
	EnabledConfigSources []string
}

// New returns a Config populated from enviroment variables.
func New() *Config {
	return &Config{
		GrpcServiceAddr:        envOrDefault("GRPC_SERVICE_ADDR", ":5010"),
		GrpcProbeAddr:          envOrDefault("GRPC_PROBE_ADDR", ":5011"),
		MetricsAddr:            envOrDefault("METRICS_ADDR", ":9090"),
		OTELExporterEndpoint:   envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		TraceSampleRatio:       envFloat64("OTEL_TRACES_SAMPLE_ARG", 0.1),
		KubeConfigPath:         os.Getenv("KUBE_CONFIG_PATH"),
		ConfigMapLabelSelector: envOrDefault("CONFIGMAP_LABEL_SELECTOR", "muninn.io/config=runtime"),
		CacheEntryTTL:          envDuration("CACHE_ENTRY_TTL", 0),
		StartupTimeout:         envDuration("STARTUP_TIMEOUT", 2*time.Minute),
		WebhookAddr:            envOrDefault("WEBHOOK_ADDR", ":8443"),
		WebhookTLSCertPath:     envOrDefault("WEBHOOK_TLS_CERT_PATH", "/etc/webhook/certs/tls.crt"),
		WebhookTLSKeyPath:      envOrDefault("WEBHOOK_TLS_KEY_PATH", "/etc/webhook/certs/tls.key"),
		SelfAddr:               envOrDefault("MUNINN_SELF_ADDR", "muninn.muninn-system.svc.cluster.local:5010"),
		InjectImage:            envOrDefault("MUNINN_INJECT_IMAGE", ""),
		EnabledConfigSources:   envCSV("ENABLED_CONFIG_SOURCES"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envFloat64(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

// envCSV splits a comma-separated env var into trimmed, non-empty entries.
// Returns nil (not an empty, non-nil slice) when the var is unset or empty,
// so callers can use len(...) == 0 to mean "no filter" unambiguously.
func envCSV(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}

	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
