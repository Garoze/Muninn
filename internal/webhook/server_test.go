package webhook

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"

	"github.com/garoze/muninn/internal/config"
)

func TestNewServer_BadCertPathErrors(t *testing.T) {
	cfg := &config.Config{
		WebhookAddr:        ":0",
		WebhookTLSCertPath: "/nonexistent/tls.crt",
		WebhookTLSKeyPath:  "/nonexistent/tls.key",
	}
	h := NewHandler(zap.NewNop(), cfg)

	_, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
	if err == nil {
		t.Fatal("expected an error for nonexistent cert/key paths, got nil")
	}
}

func TestNewServer_ValidCert_ConfiguresServer(t *testing.T) {
	certPath, keyPath := generateSelfSignedCert(t, t.TempDir())

	cfg := &config.Config{
		WebhookAddr:        "127.0.0.1:8443",
		WebhookTLSCertPath: certPath,
		WebhookTLSKeyPath:  keyPath,
	}
	h := NewHandler(zap.NewNop(), cfg)

	srv, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.Addr != cfg.WebhookAddr {
		t.Errorf("Addr: got %q, want %q", srv.Addr, cfg.WebhookAddr)
	}
	if srv.TLSConfig == nil || len(srv.TLSConfig.Certificates) != 1 {
		t.Error("expected TLSConfig with exactly one certificate loaded")
	}
}
