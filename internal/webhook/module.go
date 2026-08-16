package webhook

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"

	"github.com/garoze/muninn/internal/config"
)

var Module = fx.Options(
	fx.Provide(NewHandler),
	fx.Provide(NewServer),
	fx.Provide(NewClient),
	// First, so misconfiguration stops startup before the server binds.
	fx.Invoke(validateInjectConfig),
	fx.Invoke(startWebhookServer),
)

// validateInjectConfig rejects configuration the injected containers cannot
// work without. Without an image the API server rejects every annotated Pod for
// an empty container image, and that error is reported against the consumer's
// Pod rather than against the webhook that produced it.
func validateInjectConfig(cfg *config.Config) error {
	if cfg.InjectImage == "" {
		return fmt.Errorf("MUNINN_INJECT_IMAGE is required in webhook mode: it must match this Deployment's own image")
	}
	return nil
}

func startWebhookServer(lc fx.Lifecycle, srv *http.Server, watcher *certwatcher.CertWatcher, log *zap.Logger) {
	// The watcher reloads the certificate from disk when whoever issues it
	// rotates the mounted files. Its own context is cancelled on stop rather
	// than the start context, which is already cancelled by the time the
	// server is serving.
	watchCtx, stopWatching := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.Info("starting webhook server")
			go func() {
				if err := watcher.Start(watchCtx); err != nil {
					log.Error("webhook certificate watcher failed",
						zap.Error(err),
					)
				}
			}()

			go func() {
				// The certificate comes from the watcher via
				// TLSConfig.GetCertificate, so both args here are empty.
				if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					log.Error("webhook server failed",
						zap.Error(err),
					)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping webhook server")
			stopWatching()
			return srv.Shutdown(ctx)
		},
	})
}
