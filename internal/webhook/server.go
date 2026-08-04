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

// NewServer builds the webhook's HTTPS server. The API server always calls
// admission webhooks over TLS, so a cert/key pair is required even for
// local/dev use (see config/webhook/ for the cert-manager wiring that
// populates cfg.WebhookTLSCertPath/WebhookTLSKeyPath in-cluster). tp is
// taken as an explicit parameter, not read from otel's global
// TracerProvider, for the same reason cmd/muninn/main.go's otelgrpc wiring
// does: otelhttp.NewHandler resolves its TracerProvider once here, at
// construction, so Fx must sequence NewTracerProvider before this runs.
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
