package webhook

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/fx"
	"go.uber.org/zap"

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

func startWebhookServer(lc fx.Lifecycle, srv *http.Server, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.Info("starting webhook server")
			go func() {
				// cert/key already loaded into srv.TLSConfig by NewServer,
				// so both args here are empty.
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
			return srv.Shutdown(ctx)
		},
	})
}
