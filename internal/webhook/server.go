package webhook

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/garoze/muninn/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
)

// NewServer builds the webhook's HTTPS server. TLS is required unconditionally
// (the API server only calls admission webhooks over TLS), unlike the gRPC
// API's optional TLS - see docs/design.md. tp is an explicit parameter rather
// than otel's global provider since otelhttp resolves it once at construction.
func NewServer(cfg *config.Config, h *Handler, log *zap.Logger, tp *sdktrace.TracerProvider) (*http.Server, error) {
	cert, err := tls.LoadX509KeyPair(cfg.WebhookTLSCertPath, cfg.WebhookTLSKeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading webhook TLS cert/key: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/mutate", otelhttp.NewHandler(h, "mutate", otelhttp.WithTracerProvider(tp)))

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
		Addr:      cfg.WebhookAddr,
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}, nil
}
