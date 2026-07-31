package observability

import (
	"net"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"
	"google.golang.org/grpc/reflection"

	"github.com/garoze/muninn/internal/config"
)

// GRPCServerResult groups the gRPC server and its health server so Fx
// can consume both without multiple providers returning the same type.
type GRPCServerResult struct {
	Server       *grpc.Server
	HealthServer *health.Server
}

// NewGRPCListener binds a TCP listener on cfg.GrpcServiceAddr.
func NewGRPCListener(cfg *config.Config, log *zap.Logger) (net.Listener, error) {
	lis, err := net.Listen("tcp", cfg.GrpcServiceAddr)
	if err != nil {
		return nil, err
	}

	log.Info("gRPC listener bound",
		zap.String("addr", cfg.GrpcServiceAddr),
	)

	return lis, nil
}

// NewGRPCServer builds the main gRPC server. Health starts as NOT_SERVING;
// flip it via the returned HealthServer after cache sync.
func NewGRPCServer(log *zap.Logger, opts ...grpc.ServerOption) GRPCServerResult {
	s := grpc.NewServer(opts...)
	hs := RegisterGRPCHealth(s)
	reflection.Register(s)

	return GRPCServerResult{Server: s, HealthServer: hs}
}
