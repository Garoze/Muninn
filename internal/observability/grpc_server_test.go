package observability

import (
	"testing"

	"go.uber.org/zap"

	"github.com/garoze/muninn/internal/config"
)

func TestNewGRPCListener_BindsConfiguredAddr(t *testing.T) {
	cfg := &config.Config{GrpcServiceAddr: "127.0.0.1:0"}
	lis, err := NewGRPCListener(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer lis.Close()

	if lis.Addr().Network() != "tcp" {
		t.Errorf("network: got %q, want tcp", lis.Addr().Network())
	}
}

func TestNewGRPCListener_InvalidAddrReturnsError(t *testing.T) {
	cfg := &config.Config{GrpcServiceAddr: "not a valid address"}
	_, err := NewGRPCListener(cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for invalid bind address, got nil")
	}
}

func TestNewGRPCListener_PortAlreadyInUseReturnsError(t *testing.T) {
	cfg := &config.Config{GrpcServiceAddr: "127.0.0.1:0"}
	first, err := NewGRPCListener(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error binding first listener: %v", err)
	}
	defer first.Close()

	second := &config.Config{GrpcServiceAddr: first.Addr().String()}
	if _, err := NewGRPCListener(second, zap.NewNop()); err == nil {
		t.Fatal("expected error binding an already-in-use address, got nil")
	}
}

func TestNewGRPCServer_RegistersHealthAndReflection(t *testing.T) {
	result := NewGRPCServer(zap.NewNop())

	if result.Server == nil {
		t.Fatal("Server is nil")
	}
	if result.HealthServer == nil {
		t.Fatal("HealthServer is nil")
	}

	info := result.Server.GetServiceInfo()
	if _, ok := info["grpc.health.v1.Health"]; !ok {
		t.Error("health service not registered")
	}
	if _, ok := info["grpc.reflection.v1alpha.ServerReflection"]; !ok {
		t.Error("reflection service not registered")
	}
}

func TestNewGRPCServer_HealthStartsNotServing(t *testing.T) {
	result := NewGRPCServer(zap.NewNop())

	if got := checkStatus(t, result.HealthServer); got.String() != "NOT_SERVING" {
		t.Errorf("got %v, want NOT_SERVING", got)
	}
}
