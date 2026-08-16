package grpc

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"net"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/observability"
)

var Module = fx.Options(
	fx.Provide(newDiscoveryHandler),
	fx.Provide(NewGRPCListener),
	fx.Provide(newGRPCServer),
	fx.Invoke(startGRPCServer),
)

func newDiscoveryHandler(svc *app.DiscoveryService, metrics *observability.Metrics, log *zap.Logger) *DiscoveryHandler {
	return &DiscoveryHandler{
		Service: svc,
		Metrics: metrics,
		Logger:  log.Named("grpc"),
	}
}

func startGRPCServer(lc fx.Lifecycle, srv *grpc.Server, lis net.Listener, h *DiscoveryHandler, log *zap.Logger) {
	discoveryv1.RegisterDiscoveryServiceServer(srv, h)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.Info("starting gRPC server")
			go func() {
				if err := srv.Serve(lis); err != nil {
					log.Error("gRPC server exited",
						zap.Error(err),
					)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			log.Info("stopping gRPC server")
			srv.GracefulStop()
			return nil
		},
	})
}
