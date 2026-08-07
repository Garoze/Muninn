package envtest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	health "google.golang.org/grpc/health"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	appModule "github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	kubeModule "github.com/garoze/muninn/internal/kube"
	"github.com/garoze/muninn/internal/observability"
	webhookModule "github.com/garoze/muninn/internal/webhook"
)

// TestWebhookResolvesOwnCache_NoServeDependency proves the webhook resolves
// config from its own informer/cache, in-process, with zero dependency on
// the "serve" gRPC process. This wires the exact Fx modules the real
// "muninn webhook" binary uses (appModule.Module, kubeModule.Module,
// webhookModule.NewHandler) - no grpcTransport.Module, no gRPC dial, no
// SelfAddr lookup anywhere in this test - and asserts the resulting
// DiscoveryService resolves real cluster state, and that ServeHTTP's
// admission behavior tracks the cache's own sync state rather than any
// external server's availability.
func TestWebhookResolvesOwnCache_NoServeDependency(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}

	env := &envtest.Environment{}
	restCfg, err := env.Start()
	if err != nil {
		if isMissingBinariesError(err) {
			t.Skipf("envtest binaries not found (set KUBEBUILDER_ASSETS): %v", err)
		}
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	kubeconfigPath := writeKubeconfig(t, restCfg)

	scheme, err := kubeModule.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	k8sClient, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	// Fixtures created before the watcher starts: Watcher.Start blocks on
	// its initial list+sync, so by the time Fx's RequireStart returns below,
	// this ConfigMap is already projected - no polling needed to prove it.
	const namespace = "night-city"
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "runtime-config",
			Namespace: namespace,
			Labels:    map[string]string{"muninn.io/config": "runtime"},
		},
		Data: map[string]string{"DB_HOST": "postgres.night-city.svc"},
	}
	if err := k8sClient.Create(context.Background(), cm); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	cfg := &config.Config{
		KubeConfigPath:         kubeconfigPath,
		ConfigMapLabelSelector: "muninn.io/config=runtime",
	}

	var handler *webhookModule.Handler
	var svc *appModule.DiscoveryService

	// Mirrors cmd/muninn/main.go's startWebhook() Fx wiring exactly, minus
	// webhookModule.NewServer (no TLS listener needed - ServeHTTP is called
	// directly below) and minus anything gRPC-related, which webhook mode
	// never provides.
	fxApp := fxtest.New(t,
		fx.Provide(func() *config.Config { return cfg }),
		fx.Provide(func() *zap.Logger { return zaptest.NewLogger(t) }),
		fx.Provide(func() *observability.Metrics { return observability.NewMetrics(prometheus.NewRegistry()) }),
		// Same nil-health wiring as startWebhook: these gate serve's gRPC
		// readiness, which webhook mode has none of - MarkHealthServing
		// nil-checks both independently.
		fx.Provide(func() *health.Server { return nil }),
		fx.Provide(func() *observability.StandaloneHealth { return nil }),
		appModule.Module,
		kubeModule.Module,
		fx.Provide(webhookModule.NewClient),
		fx.Provide(webhookModule.NewHandler),
		fx.Populate(&handler, &svc),
	)
	fxApp.RequireStart()
	defer fxApp.RequireStop()

	// The core proof: DiscoveryService.Resolve reflects the real ConfigMap,
	// via the webhook's own cache - no gRPC client, no dial to "serve"
	// anywhere in this test's dependency graph.
	if !svc.Cache.IsSynced() {
		t.Fatal("webhook's own cache did not report synced after Fx start")
	}

	results, _, err := svc.Resolve(context.Background(), namespace)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got := make(map[string]any, len(results))
	for _, r := range results {
		got[r.Key] = r.Value
	}
	if got["DB_HOST"] != "postgres.night-city.svc" {
		t.Errorf("Resolve()[DB_HOST] = %v, want postgres.night-city.svc (got %v)", got["DB_HOST"], got)
	}

	// Round-trip through the real admission path: an annotated Pod in the
	// same namespace should get injected now that the cache is synced.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "netrunner",
			Annotations: map[string]string{webhookModule.InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBodyFor(t, namespace, pod)

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: body=%s", rec.Code, rec.Body.String())
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || !review.Response.Allowed {
		t.Fatalf("expected Allowed: true, got %+v", review.Response)
	}
	if review.Response.Patch == nil {
		t.Error("expected a patch once the cache is synced, got none")
	}
}

func writeKubeconfig(t *testing.T, restCfg *rest.Config) string {
	t.Helper()

	kubeCfg := clientcmdapi.NewConfig()
	kubeCfg.Clusters["envtest"] = &clientcmdapi.Cluster{
		Server:                   restCfg.Host,
		CertificateAuthorityData: restCfg.CAData,
	}
	kubeCfg.AuthInfos["envtest"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: restCfg.CertData,
		ClientKeyData:         restCfg.KeyData,
	}
	kubeCfg.Contexts["envtest"] = &clientcmdapi.Context{
		Cluster:  "envtest",
		AuthInfo: "envtest",
	}
	kubeCfg.CurrentContext = "envtest"

	path := t.TempDir() + "/kubeconfig"
	if err := clientcmd.WriteToFile(*kubeCfg, path); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

func admissionReviewBodyFor(t *testing.T, namespace string, pod *corev1.Pod) []byte {
	t.Helper()

	podJSON, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}

	review := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       types.UID("webhook-envtest"),
			Namespace: namespace,
			Object:    runtime.RawExtension{Raw: podJSON},
		},
	}

	out, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal review: %v", err)
	}
	return out
}
