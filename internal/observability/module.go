package observability

import (
	"context"
	"net/http"
	"time"

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
	fx.Provide(NewStandaloneHealth),
	fx.Provide(NewTracerProvider),
	fx.Invoke(StartMetricsServer),
	fx.Invoke(ShutdownTracerProvider),
)

// NewLogger constructs the shared production zap.Logger.
func NewLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

// StartMetricsServer registers an OnStart/OnStop hook that serves /metrics.
// Exported separately from Module so callers can start it without pulling
// in unrelated dependencies.
func StartMetricsServer(lc fx.Lifecycle, cfg *config.Config, log *zap.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	// ReadHeaderTimeout bounds a client that opens a connection and sends
	// headers slowly or never; without it such a connection is held open
	// indefinitely.
	srv := &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

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

// ShutdownTracerProvider registers an OnStop hook that shuts tp down.
// Exported separately from Module so callers can release it without pulling
// in unrelated dependencies.
func ShutdownTracerProvider(lc fx.Lifecycle, tp *sdktrace.TracerProvider) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		},
	})
}
