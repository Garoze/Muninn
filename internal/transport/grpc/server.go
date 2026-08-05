package grpc

import (
	"fmt"
	"net"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	health "google.golang.org/grpc/health"
	"google.golang.org/grpc/reflection"

	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
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
	hs := observability.RegisterGRPCHealth(s)
	reflection.Register(s)

	return GRPCServerResult{Server: s, HealthServer: hs}
}

// TLSServerOption builds a grpc.ServerOption for TLS from
// cfg.GRPCTLSCertPath/GRPCTLSKeyPath, or a nil option if both are unset -
// this server's TLS is opt-in, unlike the webhook's (see docs/design.md).
// Setting only one of the two paths is an error, not a partial default.
func TLSServerOption(cfg *config.Config) (grpc.ServerOption, error) {
	if cfg.GRPCTLSCertPath == "" && cfg.GRPCTLSKeyPath == "" {
		return nil, nil
	}

	if cfg.GRPCTLSCertPath == "" || cfg.GRPCTLSKeyPath == "" {
		return nil, fmt.Errorf("GRPC_TLS_CERT_PATH and GRPC_TLS_KEY_PATH must both be set or both be unset")
	}

	creds, err := credentials.NewServerTLSFromFile(cfg.GRPCTLSCertPath, cfg.GRPCTLSKeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading gRPC TLS cert/key: %w", err)
	}

	return grpc.Creds(creds), nil
}

// newGRPCServer builds the main *grpc.Server and *health.Server, wiring in
// OTel stats instrumentation and TLS (see TLSServerOption). Takes
// *sdktrace.TracerProvider explicitly rather than the global, since
// otelgrpc.NewServerHandler resolves it once at construction, not per call.
func newGRPCServer(cfg *config.Config, log *zap.Logger, tp *sdktrace.TracerProvider) (*grpc.Server, *health.Server, error) {
	opts := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tp))),
	}

	tlsOpt, err := TLSServerOption(cfg)
	if err != nil {
		return nil, nil, err
	}
	if tlsOpt != nil {
		opts = append(opts, tlsOpt)
	}

	r := NewGRPCServer(log, opts...)
	return r.Server, r.HealthServer, nil
}
