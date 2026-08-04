package webhook

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"

	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

// TestModule_StartsAndServesRealHTTPS builds the real Fx Module - the same
// wiring `muninn webhook` uses - generates a throwaway self-signed cert,
// starts it on a real local port, and sends it a real HTTPS request. This
// is the automated form of the manual curl verification this webhook was
// originally checked with by hand.
func TestModule_StartsAndServesRealHTTPS(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateSelfSignedCert(t, dir)
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	cfg := &config.Config{
		WebhookAddr:        addr,
		WebhookTLSCertPath: certPath,
		WebhookTLSKeyPath:  keyPath,
	}

	fxApp := fxtest.New(t,
		fx.Provide(func() *config.Config { return cfg }),
		fx.Provide(zap.NewNop),
		fx.Provide(sdktrace.NewTracerProvider),
		fx.Provide(func() *observability.Metrics { return observability.NewMetrics(prometheus.NewRegistry()) }),
		Module,
	)
	fxApp.RequireStart()
	defer fxApp.RequireStop()

	// ListenAndServeTLS runs in its own goroutine inside the OnStart hook,
	// so RequireStart returning doesn't guarantee the listener is actually
	// accepting connections yet.
	waitForServer(t, addr)

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		Timeout:   2 * time.Second,
	}

	body := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test-uid"}}`
	resp, err := client.Post("https://"+addr+"/mutate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /mutate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(resp.Body).Decode(&review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || !review.Response.Allowed || string(review.Response.UID) != "test-uid" {
		t.Errorf("got %+v", review.Response)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s never became ready", addr)
}

// generateSelfSignedCert writes a throwaway self-signed cert/key pair to
// dir, returning their paths. NewServer requires real files on disk
// (tls.LoadX509KeyPair), so anything exercising it needs something written
// out, not just in-memory bytes.
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
