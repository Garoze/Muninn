package discoveryclient

import (
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
)

// grpc.NewClient defers connection establishment to the first RPC, so there's
// no "invalid target" case to test here - Dial's error only fires if
// TLSDialOption fails loading caPath.
func TestDial_PlaintextReturnsUsableClientAndConn(t *testing.T) {
	client, conn, err := Dial("localhost:5010", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if client == nil {
		t.Error("expected a non-nil DiscoveryServiceClient")
	}
	if conn == nil {
		t.Error("expected a non-nil *grpc.ClientConn")
	}
}

func TestDial_BadCAPath_Errors(t *testing.T) {
	_, _, err := Dial("localhost:5010", "/nonexistent/ca.crt")
	if err == nil {
		t.Fatal("expected an error for a nonexistent CA path")
	}
}

func TestTLSDialOption_EmptyPath_ReturnsInsecureCredentials(t *testing.T) {
	opt, err := TLSDialOption("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt == nil {
		t.Fatal("expected a non-nil grpc.DialOption for plaintext")
	}
}

func TestTLSDialOption_NonexistentPath_Errors(t *testing.T) {
	_, err := TLSDialOption("/nonexistent/ca.crt")
	if err == nil {
		t.Fatal("expected an error for a nonexistent CA path")
	}
}

func TestTLSDialOption_ValidCA_ReturnsCredsOption(t *testing.T) {
	caPath := generateSelfSignedCA(t, t.TempDir())

	opt, err := TLSDialOption(caPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt == nil {
		t.Fatal("expected a non-nil grpc.DialOption for a valid CA")
	}
}

// generateSelfSignedCA writes a throwaway self-signed CA cert to dir and
// returns its path, since credentials.NewClientTLSFromFile requires a real
// file on disk.
func generateSelfSignedCA(t *testing.T, dir string) (caPath string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	caPath = filepath.Join(dir, "ca.crt")

	certOut, err := os.Create(caPath)
	if err != nil {
		t.Fatalf("create ca file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		_ = certOut.Close()
		t.Fatalf("encode ca: %v", err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatalf("close ca file: %v", err)
	}

	return caPath
}
