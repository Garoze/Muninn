package envtest_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/kube"
	"github.com/garoze/muninn/internal/observability"
)

// TestTwoSourcesOfSameKind is the case ADR-0008 describes: two ConfigMap
// sources in one namespace scoped by different label selectors, expressing a
// base configuration layer and a per-feature override layer.
//
// It needs a real API server because the failure it guards is in informer
// construction, not in the merge. Both sources resolve to the same GVK, so a
// single shared cache serves them one informer and one label selector, and the
// other selector is silently dropped. The unit-level same-Kind test uses a fake
// source with no informer at all, which is exactly why it cannot catch this.
func TestTwoSourcesOfSameKind(t *testing.T) {
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

	scheme, err := kube.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}
	k8sClient, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	appCache := app.NewCache()
	sources := []kube.ConfigSource{
		kube.NewConfigMapSource(&config.Config{},
			kube.WithLabelSelector("muninn.io/config-tier=base"),
			kube.WithKeyPrefix("ConfigMap:base"),
		),
		kube.NewConfigMapSource(&config.Config{},
			kube.WithLabelSelector("muninn.io/config-tier=feature"),
			kube.WithKeyPrefix("ConfigMap:feature"),
		),
	}

	w, err := kube.NewWatcher(restCfg, scheme, appCache,
		observability.NewMetrics(prometheus.NewRegistry()), nil, nil, zaptest.NewLogger(t), sources)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := w.Start(startCtx); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	t.Cleanup(w.Stop)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "arasaka"}}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	base := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config",
			Namespace: "arasaka",
			Labels:    map[string]string{"muninn.io/config-tier": "base"},
		},
		Data: map[string]string{"LOG_LEVEL": "info", "TIMEOUT": "30s"},
	}
	if err := k8sClient.Create(context.Background(), base); err != nil {
		t.Fatalf("create base configmap: %v", err)
	}

	feature := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config-feature",
			Namespace: "arasaka",
			Labels:    map[string]string{"muninn.io/config-tier": "feature"},
		},
		Data: map[string]string{"FEATURE_X": "true"},
	}
	if err := k8sClient.Create(context.Background(), feature); err != nil {
		t.Fatalf("create feature configmap: %v", err)
	}

	t.Run("both layers reach the cache under their own identity", func(t *testing.T) {
		eventually(t, 15*time.Second, func() bool {
			e := appCache.Get("arasaka")
			if e == nil {
				return false
			}
			merged := e.Merged()
			return merged["LOG_LEVEL"] == "info" && merged["FEATURE_X"] == "true"
		}, "both label selectors must be honored; one source's selector was dropped")

		e := appCache.Get("arasaka")
		if got := e.Sources["ConfigMap:base/app-config"]["LOG_LEVEL"]; got != "info" {
			t.Errorf("base layer slice: got %v, want info (sources: %+v)", got, e.Sources)
		}
		if got := e.Sources["ConfigMap:feature/app-config-feature"]["FEATURE_X"]; got != "true" {
			t.Errorf("feature layer slice: got %v, want true (sources: %+v)", got, e.Sources)
		}
	})

	t.Run("each source watches only its own selector", func(t *testing.T) {
		e := appCache.Get("arasaka")

		// If the two sources shared one informer, whichever selector survived
		// would feed both, and each object would appear under both prefixes.
		if _, ok := e.Sources["ConfigMap:base/app-config-feature"]; ok {
			t.Errorf("feature ConfigMap leaked into the base source: %+v", e.Sources)
		}
		if _, ok := e.Sources["ConfigMap:feature/app-config"]; ok {
			t.Errorf("base ConfigMap leaked into the feature source: %+v", e.Sources)
		}
	})
}
