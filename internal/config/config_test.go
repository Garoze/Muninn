package config

import (
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	cfg := New()

	cases := map[string]struct {
		got  any
		want any
	}{
		"GrpcServiceAddr":        {cfg.GrpcServiceAddr, ":5010"},
		"GrpcProbeAddr":          {cfg.GrpcProbeAddr, ":5011"},
		"MetricsAddr":            {cfg.MetricsAddr, ":9090"},
		"OTELExporterEndpoint":   {cfg.OTELExporterEndpoint, "localhost:4317"},
		"TraceSampleRatio":       {cfg.TraceSampleRatio, 0.1},
		"KubeConfigPath":         {cfg.KubeConfigPath, ""},
		"ConfigMapLabelSelector": {cfg.ConfigMapLabelSelector, "muninn.io/config=runtime"},
		"CacheEntryTTL":          {cfg.CacheEntryTTL, time.Duration(0)},
		"StartupTimeout":         {cfg.StartupTimeout, 2 * time.Minute},
		"SelfAddr":               {cfg.SelfAddr, "muninn.muninn-system.svc.cluster.local:5010"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s: got %v, want %v", name, tc.got, tc.want)
			}
		})
	}
}

func TestNew_EnvOverrides(t *testing.T) {
	t.Setenv("GRPC_SERVICE_ADDR", ":9999")
	t.Setenv("GRPC_PROBE_ADDR", ":9998")
	t.Setenv("METRICS_ADDR", ":9997")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel.example:4317")
	t.Setenv("OTEL_TRACES_SAMPLE_ARG", "0.75")
	t.Setenv("KUBE_CONFIG_PATH", "/tmp/kubeconfig")
	t.Setenv("CONFIGMAP_LABEL_SELECTOR", "app.io/config=true")
	t.Setenv("CACHE_ENTRY_TTL", "30s")
	t.Setenv("STARTUP_TIMEOUT", "5m")
	t.Setenv("MUNINN_SELF_ADDR", "muninn.other-ns.svc.cluster.local:5010")

	cfg := New()

	if cfg.GrpcServiceAddr != ":9999" {
		t.Errorf("GrpcServiceAddr: got %q", cfg.GrpcServiceAddr)
	}
	if cfg.GrpcProbeAddr != ":9998" {
		t.Errorf("GrpcProbeAddr: got %q", cfg.GrpcProbeAddr)
	}
	if cfg.MetricsAddr != ":9997" {
		t.Errorf("MetricsAddr: got %q", cfg.MetricsAddr)
	}
	if cfg.OTELExporterEndpoint != "otel.example:4317" {
		t.Errorf("OTELExporterEndpoint: got %q", cfg.OTELExporterEndpoint)
	}
	if cfg.TraceSampleRatio != 0.75 {
		t.Errorf("TraceSampleRatio: got %v", cfg.TraceSampleRatio)
	}
	if cfg.KubeConfigPath != "/tmp/kubeconfig" {
		t.Errorf("KubeConfigPath: got %q", cfg.KubeConfigPath)
	}
	if cfg.ConfigMapLabelSelector != "app.io/config=true" {
		t.Errorf("ConfigMapLabelSelector: got %q", cfg.ConfigMapLabelSelector)
	}
	if cfg.CacheEntryTTL != 30*time.Second {
		t.Errorf("CacheEntryTTL: got %v", cfg.CacheEntryTTL)
	}
	if cfg.StartupTimeout != 5*time.Minute {
		t.Errorf("StartupTimeout: got %v", cfg.StartupTimeout)
	}
	if cfg.SelfAddr != "muninn.other-ns.svc.cluster.local:5010" {
		t.Errorf("SelfAddr: got %q", cfg.SelfAddr)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_STR", "value")
		if got := envOrDefault("MUNINN_TEST_STR", "fallback"); got != "value" {
			t.Errorf("got %q, want %q", got, "value")
		}
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		if got := envOrDefault("MUNINN_TEST_STR_UNSET", "fallback"); got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})

	t.Run("returns fallback when set to empty string", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_STR_EMPTY", "")
		if got := envOrDefault("MUNINN_TEST_STR_EMPTY", "fallback"); got != "fallback" {
			t.Errorf("got %q, want %q", got, "fallback")
		}
	})
}

func TestEnvFloat64(t *testing.T) {
	t.Run("returns parsed value when set", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_FLOAT", "0.42")
		if got := envFloat64("MUNINN_TEST_FLOAT", 1.0); got != 0.42 {
			t.Errorf("got %v, want %v", got, 0.42)
		}
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		if got := envFloat64("MUNINN_TEST_FLOAT_UNSET", 1.0); got != 1.0 {
			t.Errorf("got %v, want %v", got, 1.0)
		}
	})

	t.Run("returns fallback when invalid", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_FLOAT_INVALID", "not-a-float")
		if got := envFloat64("MUNINN_TEST_FLOAT_INVALID", 1.0); got != 1.0 {
			t.Errorf("got %v, want %v", got, 1.0)
		}
	})
}

func TestEnvDuration(t *testing.T) {
	t.Run("returns parsed value when set", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_DURATION", "10s")
		if got := envDuration("MUNINN_TEST_DURATION", time.Minute); got != 10*time.Second {
			t.Errorf("got %v, want %v", got, 10*time.Second)
		}
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		if got := envDuration("MUNINN_TEST_DURATION_UNSET", time.Minute); got != time.Minute {
			t.Errorf("got %v, want %v", got, time.Minute)
		}
	})

	t.Run("returns fallback when invalid", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_DURATION_INVALID", "not-a-duration")
		if got := envDuration("MUNINN_TEST_DURATION_INVALID", time.Minute); got != time.Minute {
			t.Errorf("got %v, want %v", got, time.Minute)
		}
	})
}
