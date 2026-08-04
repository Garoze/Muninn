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
	fx.Provide(NewLogger),
	fx.Provide(func() prometheus.Registerer { return prometheus.DefaultRegisterer }),
	fx.Provide(NewMetrics),
	fx.Provide(NewGRPCListener),
	fx.Provide(NewStandaloneHealth),
	fx.Provide(NewTracerProvider),
	fx.Invoke(StartMetricsServer),
	fx.Invoke(ShutdownTracerProvider),
)

// NewLogger constructs the shared production zap.Logger, used by every
// muninn process mode (serve, webhook).
func NewLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

// StartMetricsServer registers an OnStart/OnStop hook that serves /metrics,
// so both serve (via Module) and webhook (called directly, to avoid
// pulling in the gRPC-listener/tracer-provider pieces webhook mode doesn't
// need from Module) expose it the same way.
func StartMetricsServer(lc fx.Lifecycle, cfg *config.Config, log *zap.Logger) {
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

// ShutdownTracerProvider registers an OnStop hook that shuts tp down,
// so both serve (via Module) and webhook (called directly, to avoid
// pulling in the pieces webhook mode doesn't need) release it the same way.
func ShutdownTracerProvider(lc fx.Lifecycle, tp *sdktrace.TracerProvider) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		},
	})
}
