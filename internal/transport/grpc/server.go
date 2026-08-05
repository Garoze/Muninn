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

// TLSServerOption builds a grpc.ServerOption enabling TLS on the gRPC API
// from cfg.GRPCTLSCertPath/GRPCTLSKeyPath, or returns a nil option when
// both are unset - the gRPC API's TLS posture is opt-in, unlike the
// webhook's (see config.Config.WebhookTLSCertPath): some deployments
// terminate mTLS at a service mesh sidecar and want this server to stay
// plaintext, others don't run behind a mesh and need TLS terminated here
// directly. Setting only one of the two paths is a configuration error,
// not a partial default, since a cert without its key (or vice versa)
// can't produce a usable TLS config either way.
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

// newGRPCServer builds the main *grpc.Server and its *health.Server,
// wiring in OTel gRPC stats instrumentation and TLS (opt-in, see
// TLSServerOption). Depends on *sdktrace.TracerProvider explicitly (not
// otel.GetTracerProvider()'s global) so Fx sequences NewTracerProvider
// before this runs - otelgrpc.NewServerHandler resolves its TracerProvider
// once at construction, not lazily per request.
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
