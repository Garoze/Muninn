package observability

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/garoze/muninn/internal/config"
)

var Module = fx.Options(
	fx.Provide(newLogger),
	fx.Provide(func() prometheus.Registerer { return prometheus.DefaultRegisterer }),
	fx.Provide(NewMetrics),
	fx.Provide(NewGRPCListener),
	fx.Provide(NewStandaloneHealth),
	fx.Provide(NewTracerProvider),
	fx.Invoke(startMetricsServer),
	fx.Invoke(shutdownTracerProvider),
)

func newLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

func startMetricsServer(lc fx.Lifecycle, cfg *config.Config, log *zap.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: cfg.MetricsAddr, Handler: mux}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("starting metrics server",
				zap.String("addr", cfg.MetricsAddr),
			)

			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Error("metrics server failed",
						zap.Error(err),
					)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}

func shutdownTracerProvider(lc fx.Lifecycle, tp *sdktrace.TracerProvider) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		},
	})
}
