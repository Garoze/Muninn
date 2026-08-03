package config

import (
	"os"
	"strconv"
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

	// CacheEntryTTL controls staleness enforcement for tenant cache entries.
	// Zero disables stale-entry rejection.
	CacheEntryTTL time.Duration

	// StartupTimeout is how long the Fx application is given to complete its
	// OnStart hooks, including informer cache sync. Defaults to 2 minutes.
	StartupTimeout time.Duration
}

// New returns a Config populated from enviroment variables.
func New() *Config {
	return &Config{
		GrpcServiceAddr:      envOrDefault("GRPC_SERVICE_ADDR", ":5010"),
		GrpcProbeAddr:        envOrDefault("GRPC_PROBE_ADDR", ":5011"),
		MetricsAddr:          envOrDefault("METRICS_ADDR", ":9090"),
		OTELExporterEndpoint: envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		TraceSampleRatio:     envFloat64("OTEL_TRACES_SAMPLE_ARG", 0.1),
		KubeConfigPath:       os.Getenv("KUBE_CONFIG_PATH"),
		CacheEntryTTL:        envDuration("CACHE_ENTRY_TTL", 0),
		StartupTimeout:       envDuration("STARTUP_TIMEOUT", 2*time.Minute),
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
