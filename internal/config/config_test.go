package config

import (
	"os"
	"testing"
	"time"
)

// configEnv is every variable New reads. TestNew_Defaults clears all of them,
// because a developer with any one exported - GRPC_SERVICE_ADDR while running
// two resolvers, say - would otherwise see this test fail for a reason that
// has nothing to do with the code.
var configEnv = []string{
	"GRPC_SERVICE_ADDR", "GRPC_PROBE_ADDR", "GRPC_TLS_CERT_PATH", "GRPC_TLS_KEY_PATH",
	"METRICS_ADDR", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_TRACES_SAMPLE_ARG",
	"KUBE_CONFIG_PATH", "CONFIGMAP_LABEL_SELECTOR", "ENABLED_CONFIG_SOURCES",
	"CACHE_ENTRY_TTL", "STARTUP_TIMEOUT", "WEBHOOK_ADDR", "WEBHOOK_TLS_CERT_PATH",
	"WEBHOOK_TLS_KEY_PATH", "MUNINN_INJECT_IMAGE", "MUNINN_SELF_ADDR",
	"SECRET_SPC_MODE", "VAULT_ADDRESS", "VAULT_ROLE_NAME",
}

func TestNew_Defaults(t *testing.T) {
	for _, name := range configEnv {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}

	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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
		"GRPCTLSCertPath":        {cfg.GRPCTLSCertPath, ""},
		"GRPCTLSKeyPath":         {cfg.GRPCTLSKeyPath, ""},
		"SecretSPCMode":          {cfg.SecretSPCMode, SecretSPCModeCreate},
		"VaultAddress":           {cfg.VaultAddress, "http://vault.kube-system:8200"},
		"VaultRoleName":          {cfg.VaultRoleName, "muninn"},
		"WebhookAddr":            {cfg.WebhookAddr, ":8443"},
		// Where the chart mounts the webhook's serving certificate.
		"WebhookTLSCertPath": {cfg.WebhookTLSCertPath, "/etc/webhook/certs/tls.crt"},
		"WebhookTLSKeyPath":  {cfg.WebhookTLSKeyPath, "/etc/webhook/certs/tls.key"},
		// No default is possible: it has to match the webhook's own Deployment
		// image, which the process cannot read for itself. Webhook mode
		// rejects an empty value at startup instead of guessing.
		"InjectImage": {cfg.InjectImage, ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s: got %v, want %v", name, tc.got, tc.want)
			}
		})
	}

	if cfg.EnabledConfigSources != nil {
		t.Errorf("EnabledConfigSources: got %v, want nil (no filter by default)", cfg.EnabledConfigSources)
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
	t.Setenv("ENABLED_CONFIG_SOURCES", "ConfigMap, CustomSource")
	t.Setenv("GRPC_TLS_CERT_PATH", "/etc/muninn/grpc-tls/tls.crt")
	t.Setenv("GRPC_TLS_KEY_PATH", "/etc/muninn/grpc-tls/tls.key")
	t.Setenv("SECRET_SPC_MODE", "Reference")
	t.Setenv("VAULT_ADDRESS", "http://vault.other-ns.svc.cluster.local:8200")
	t.Setenv("VAULT_ROLE_NAME", "other-role")

	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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
	wantSources := []string{"ConfigMap", "CustomSource"}
	if !slicesEqual(cfg.EnabledConfigSources, wantSources) {
		t.Errorf("EnabledConfigSources: got %v, want %v", cfg.EnabledConfigSources, wantSources)
	}
	if cfg.GRPCTLSCertPath != "/etc/muninn/grpc-tls/tls.crt" {
		t.Errorf("GRPCTLSCertPath: got %q", cfg.GRPCTLSCertPath)
	}
	if cfg.GRPCTLSKeyPath != "/etc/muninn/grpc-tls/tls.key" {
		t.Errorf("GRPCTLSKeyPath: got %q", cfg.GRPCTLSKeyPath)
	}
	if cfg.SecretSPCMode != SecretSPCModeReference {
		t.Errorf("SecretSPCMode: got %q, want %q", cfg.SecretSPCMode, SecretSPCModeReference)
	}
	if cfg.VaultAddress != "http://vault.other-ns.svc.cluster.local:8200" {
		t.Errorf("VaultAddress: got %q", cfg.VaultAddress)
	}
	if cfg.VaultRoleName != "other-role" {
		t.Errorf("VaultRoleName: got %q", cfg.VaultRoleName)
	}
}

// TestNew_SecretSPCMode_CaseInsensitiveNormalization: a typo here silently
// granting the more privileged Create mode instead of the requested
// Reference mode would be a security-relevant misconfiguration with no
// visible symptom, so SECRET_SPC_MODE is normalized case-insensitively to
// the canonical constant rather than compared/stored verbatim.
func TestNew_SecretSPCMode_CaseInsensitiveNormalization(t *testing.T) {
	cases := map[string]SecretSPCMode{
		"reference": SecretSPCModeReference,
		"REFERENCE": SecretSPCModeReference,
		"Reference": SecretSPCModeReference,
		"create":    SecretSPCModeCreate,
		"CREATE":    SecretSPCModeCreate,
		"Create":    SecretSPCModeCreate,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Setenv("SECRET_SPC_MODE", in)
			cfg, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if cfg.SecretSPCMode != want {
				t.Errorf("SecretSPCMode: got %q, want canonical %q", cfg.SecretSPCMode, want)
			}
		})
	}
}

func TestNew_SecretSPCMode_InvalidValue_Errors(t *testing.T) {
	t.Setenv("SECRET_SPC_MODE", "referance") // realistic typo, not a random string
	_, err := New()
	if err == nil {
		t.Fatal("expected an error for an invalid SECRET_SPC_MODE, got nil")
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func TestEnvCSV(t *testing.T) {
	t.Run("returns nil when unset", func(t *testing.T) {
		if got := envCSV("MUNINN_TEST_CSV_UNSET"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("returns nil when set to empty string", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_CSV_EMPTY", "")
		if got := envCSV("MUNINN_TEST_CSV_EMPTY"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("splits and trims entries", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_CSV", "a, b ,c")
		got := envCSV("MUNINN_TEST_CSV")
		want := []string{"a", "b", "c"}
		if !slicesEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("drops empty entries from trailing/double commas", func(t *testing.T) {
		t.Setenv("MUNINN_TEST_CSV_DIRTY", "a,,b,")
		got := envCSV("MUNINN_TEST_CSV_DIRTY")
		want := []string{"a", "b"}
		if !slicesEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
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
