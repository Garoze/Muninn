package webhook

import (
	"bytes"
	"crypto/tls"
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

	_, _, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
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

	srv, _, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.Addr != cfg.WebhookAddr {
		t.Errorf("Addr: got %q, want %q", srv.Addr, cfg.WebhookAddr)
	}
	// The certificate is served through the watcher rather than baked into
	// Certificates, so that a rotation on disk reaches new handshakes without
	// a restart. Asserting the callback resolves is what proves the wiring;
	// a non-empty Certificates slice would prove the opposite.
	if srv.TLSConfig == nil || srv.TLSConfig.GetCertificate == nil {
		t.Fatal("expected TLSConfig with a GetCertificate callback")
	}
	if len(srv.TLSConfig.Certificates) != 0 {
		t.Error("expected no statically loaded certificates")
	}
	cert, err := srv.TLSConfig.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert == nil || len(cert.Certificate) == 0 {
		t.Error("GetCertificate returned no certificate")
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

	srv, _, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
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

// The reason the certificate is served through a watcher at all: whoever
// issues it rotates it before expiry by rewriting the same mounted files, and
// a process serving the copy it read at startup keeps presenting the old one
// until it restarts. Past expiry the API server's handshake fails, and under
// failurePolicy: Fail that blocks Pod creation across every namespace this
// webhook matches.
func TestNewServer_ServesRotatedCertificateWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateSelfSignedCert(t, dir)
	cfg := &config.Config{
		WebhookAddr:        "127.0.0.1:0",
		WebhookTLSCertPath: certPath,
		WebhookTLSKeyPath:  keyPath,
	}
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h := NewHandler(zap.NewNop(), cfg, observability.NewMetrics(prometheus.NewRegistry()), svc, c)

	srv, watcher, err := NewServer(cfg, h, zap.NewNop(), sdktrace.NewTracerProvider())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	before, err := srv.TLSConfig.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate before rotation: %v", err)
	}

	// Rewrite both files in place, as a rotation does, then have the watcher
	// re-read them. Reading directly keeps the test off the filesystem-event
	// timing that the watcher's own goroutine depends on.
	generateSelfSignedCert(t, dir)
	if err := watcher.ReadCertificate(); err != nil {
		t.Fatalf("re-read rotated certificate: %v", err)
	}

	after, err := srv.TLSConfig.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate after rotation: %v", err)
	}

	if bytes.Equal(before.Certificate[0], after.Certificate[0]) {
		t.Error("server still presents the certificate it started with after rotation")
	}
}
