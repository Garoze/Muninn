package webhook

import (
	"context"
	"net/http"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Options(
	fx.Provide(NewHandler),
	fx.Provide(NewServer),
	fx.Invoke(startWebhookServer),
)

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
