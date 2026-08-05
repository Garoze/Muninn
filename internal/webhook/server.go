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
