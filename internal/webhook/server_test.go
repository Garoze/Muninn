package webhook

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

func TestNewServer_BadCertPathErrors(t *testing.T) {
	cfg := &config.Config{
		WebhookAddr:        ":0",
		WebhookTLSCertPath: "/nonexistent/tls.crt",
		WebhookTLSKeyPath:  "/nonexistent/tls.key",
	}
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h := NewHandler(zap.NewNop(), cfg, observability.NewMetrics(prometheus.NewRegistry()), svc, c)

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
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h := NewHandler(zap.NewNop(), cfg, observability.NewMetrics(prometheus.NewRegistry()), svc, c)

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

// Readiness has to track cache sync, not just process liveness: an unsynced
// replica admits annotated Pods without injecting into them, so routing to it
// silently drops injection for every Pod created in that window.
func TestNewServer_ReadyzTracksCacheSync(t *testing.T) {
	certPath, keyPath := generateSelfSignedCert(t, t.TempDir())
	cfg := &config.Config{
		WebhookAddr:        "127.0.0.1:0",
		WebhookTLSCertPath: certPath,
		WebhookTLSKeyPath:  keyPath,
	}
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h := NewHandler(zap.NewNop(), cfg, observability.NewMetrics(prometheus.NewRegistry()), svc, c)

	srv, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	get := func(path string) int {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}

	if got := get("/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("/readyz before sync: got %d, want %d", got, http.StatusServiceUnavailable)
	}
	// Liveness must not depend on sync, or a slow initial list restarts the
	// process instead of just holding it out of traffic.
	if got := get("/healthz"); got != http.StatusOK {
		t.Errorf("/healthz before sync: got %d, want %d", got, http.StatusOK)
	}

	svc.Cache.SetSynced()

	if got := get("/readyz"); got != http.StatusOK {
		t.Errorf("/readyz after sync: got %d, want %d", got, http.StatusOK)
	}
}
