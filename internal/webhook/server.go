package webhook

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/garoze/muninn/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

// maxAdmissionBody bounds a single AdmissionReview. Real ones are a few
// hundred kilobytes at most, and this endpoint is reachable by anything that
// can route to the Service, so an unbounded body is a way to exhaust a
// component whose unavailability blocks Pod creation cluster-wide.
const maxAdmissionBody = 4 << 20 // 4 MiB

// NewServer builds the webhook's HTTPS server. TLS is required unconditionally
// (the API server only calls admission webhooks over TLS), unlike the gRPC
// API's optional TLS - see docs/design.md. tp is an explicit parameter rather
// than otel's global provider since otelhttp resolves it once at construction.
//
// The certificate is served through a watcher rather than loaded once: whoever
// issues it rotates it well before expiry and writes the new one to the same
// mounted paths, and a process holding the copy it read at startup keeps
// serving the old one until it restarts. Past expiry the API server's
// handshake fails, and under failurePolicy: Fail that blocks Pod creation
// across every namespace this webhook matches.
func NewServer(cfg *config.Config, h *Handler, log *zap.Logger, tp *sdktrace.TracerProvider) (*http.Server, *certwatcher.CertWatcher, error) {
	watcher, err := certwatcher.New(cfg.WebhookTLSCertPath, cfg.WebhookTLSKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("watching webhook TLS cert/key: %w", err)
	}

	mux := http.NewServeMux()
	mutate := http.MaxBytesHandler(h, maxAdmissionBody)
	mux.Handle("/mutate", otelhttp.NewHandler(mutate, "mutate", otelhttp.WithTracerProvider(tp)))

	// Liveness only proves the TLS listener is up. It must not depend on cache
	// sync, or a slow initial list would restart the process instead of just
	// holding it out of traffic.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Readiness additionally requires a synced config cache. Until then the
	// handler admits annotated Pods without injecting anything, so routing to
	// this replica would silently skip injection for every Pod created in the
	// window.
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !h.svc.Cache.IsSynced() {
			http.Error(w, "config cache not synced", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	log.Info("webhook TLS server configured",
		zap.String("addr", cfg.WebhookAddr),
		zap.String("cert_path", cfg.WebhookTLSCertPath),
	)

	return &http.Server{
		Addr:              cfg.WebhookAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         &tls.Config{GetCertificate: watcher.GetCertificate},
	}, watcher, nil
}
