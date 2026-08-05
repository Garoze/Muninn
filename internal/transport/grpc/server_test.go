package grpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	health "google.golang.org/grpc/health"

	"github.com/garoze/muninn/internal/config"
)

func checkStatus(t *testing.T, hs *health.Server) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := hs.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return resp.Status
}

func TestNewGRPCListener_BindsConfiguredAddr(t *testing.T) {
	cfg := &config.Config{GrpcServiceAddr: "127.0.0.1:0"}
	lis, err := NewGRPCListener(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = lis.Close() }()

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
	defer func() { _ = first.Close() }()

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

func TestTLSServerOption_BothUnset_ReturnsNilOption(t *testing.T) {
	opt, err := TLSServerOption(&config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt != nil {
		t.Errorf("expected a nil option (plaintext) when both paths are unset, got %v", opt)
	}
}

func TestTLSServerOption_OnlyCertSet_Errors(t *testing.T) {
	_, err := TLSServerOption(&config.Config{GRPCTLSCertPath: "/some/tls.crt"})
	if err == nil {
		t.Fatal("expected an error when only GRPCTLSCertPath is set")
	}
}

func TestTLSServerOption_OnlyKeySet_Errors(t *testing.T) {
	_, err := TLSServerOption(&config.Config{GRPCTLSKeyPath: "/some/tls.key"})
	if err == nil {
		t.Fatal("expected an error when only GRPCTLSKeyPath is set")
	}
}

func TestTLSServerOption_BadCertPath_Errors(t *testing.T) {
	_, err := TLSServerOption(&config.Config{
		GRPCTLSCertPath: "/nonexistent/tls.crt",
		GRPCTLSKeyPath:  "/nonexistent/tls.key",
	})
	if err == nil {
		t.Fatal("expected an error for nonexistent cert/key paths")
	}
}

func TestTLSServerOption_ValidCert_ReturnsCredsOption(t *testing.T) {
	certPath, keyPath := generateSelfSignedCert(t, t.TempDir())

	opt, err := TLSServerOption(&config.Config{
		GRPCTLSCertPath: certPath,
		GRPCTLSKeyPath:  keyPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt == nil {
		t.Fatal("expected a non-nil grpc.Creds option for a valid cert/key pair")
	}
}

// generateSelfSignedCert writes a throwaway self-signed cert/key pair to
// dir, returning their paths. credentials.NewServerTLSFromFile requires
// real files on disk, so anything exercising it needs something written
// out, not just in-memory bytes. Mirrors internal/webhook/module_test.go's
// helper of the same name/shape - not shared across packages since both
// are test-only and internal/webhook must not import internal/transport/grpc
// just for this.
func generateSelfSignedCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")

	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		_ = certOut.Close()
		t.Fatalf("encode cert: %v", err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatalf("close cert file: %v", err)
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		_ = keyOut.Close()
		t.Fatalf("encode key: %v", err)
	}
	if err := keyOut.Close(); err != nil {
		t.Fatalf("close key file: %v", err)
	}

	return certPath, keyPath
}
