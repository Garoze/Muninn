package observability

import (
	"context"
	"fmt"
	"net"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/garoze/muninn/internal/config"
)

// StandaloneHealth wraps the dedicated probe health server, giving it a
// distinct type in the Fx graph to avoid collision with the main *health.Server.
type StandaloneHealth struct {
	Server *health.Server
}

// RegisterGRPCHealth registers the gRPC health service on the giver server.
// Starts as NOT_SERVING - call MarkHealthServing once the app is ready.
func RegisterGRPCHealth(s *grpc.Server) *health.Server {
	hs := health.NewServer()
	healthpb.RegisterHealthServer(s, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	return hs
}

// MarkHealthServing flips both the main and standalone probe health servers
// to SERVING. Call once the informer cache completes its initial sync.
func MarkHealthServing(mainHS *health.Server, probeHS *StandaloneHealth) {
	if mainHS != nil {
		mainHS.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}

	if probeHS != nil && probeHS.Server != nil {
		probeHS.Server.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}
}

// NewStandaloneHealth starts a dedicated gRPC health probe server on
// cfg.GrpcProbeAddr and registers its lifecycle with Fx.
func NewStandaloneHealth(lc fx.Lifecycle, cfg *config.Config, log *zap.Logger) *StandaloneHealth {
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	s := grpc.NewServer()
	healthpb.RegisterHealthServer(s, hs)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			lis, err := net.Listen("tcp", cfg.GrpcProbeAddr)
			if err != nil {
				return fmt.Errorf("gRPC health probe listen %s: %w", cfg.GrpcProbeAddr, err)
			}

			log.Info("gRPC health probe server listening",
				zap.String("addr", cfg.GrpcProbeAddr),
			)

			go func() {
				if err := s.Serve(lis); err != nil {
					log.Error("gRPC health probe server exited",
						zap.String("addr", cfg.GrpcProbeAddr),
					)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping gRPC health probe server")
			s.GracefulStop()
			return nil
		},
	})

	return &StandaloneHealth{Server: hs}
}
