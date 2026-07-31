package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"

	appModule "github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	kubeModule "github.com/garoze/muninn/internal/kube"
	"github.com/garoze/muninn/internal/observability"
	grpcTransport "github.com/garoze/muninn/internal/transport/grpc"
)

func startFx() (*fx.App, error) {
	app := fx.New(
		fx.Provide(config.New),
		observability.Module,
		appModule.Module,
		kubeModule.Module,
		grpcTransport.Module,

		// gRPC server - constructed here (composition root) to avoid an
		// import cycle between observability (listener/health) and
		// transport/grpc (handler registration)
		fx.Provide(func(log *zap.Logger) (*grpc.Server, *health.Server) {
			r := observability.NewGRPCServer(log)
			return r.Server, r.HealthServer
		}),

		fx.Invoke(func(lc fx.Lifecycle, log *zap.Logger) {
			lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					log.Info("shutting down")
					return nil
				},
			})
		}),
	)

	cfg := config.New()
	startCtx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return nil, err
	}

	return app, nil
}

func main() {
	app, err := startFx()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] failed to start: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.Stop(stopCtx); err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] failed to stop cleanly: %v\n", err)
		os.Exit(1)
	}
}
