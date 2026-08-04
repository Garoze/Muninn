package envtest_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/kube"
	"github.com/garoze/muninn/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestWatcherProjection(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}

	// ConfigMap is a core Kubernetes type - no CRDs need installing for
	// envtest's kube-apiserver to know about it.
	env := &envtest.Environment{}

	cfg, err := env.Start()
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

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	appCache := app.NewCache()
	metrics := observability.NewMetrics(prometheus.NewRegistry())
	log := zaptest.NewLogger(t)

	w, err := kube.NewWatcher(cfg, scheme, appCache, metrics, nil, nil, log, &config.Config{
		ConfigMapLabelSelector: "muninn.io/config=runtime",
	})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startCancel()

	if err := w.Start(startCtx); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	t.Cleanup(w.Stop)

	// Create namespace and fixtures: two ConfigMaps in the same namespace,
	// to prove per-source ownership of the patch-based merge.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "arasaka"}}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	cmA := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "runtime-config",
			Namespace: "arasaka",
			Labels:    map[string]string{"muninn.io/config": "runtime"},
		},
		Data: map[string]string{"LOG_LEVEL": "info"},
	}
	if err := k8sClient.Create(context.Background(), cmA); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	cmB := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "feature-flags",
			Namespace: "arasaka",
			Labels:    map[string]string{"muninn.io/config": "runtime"},
		},
		Data: map[string]string{"DARK_MODE": "true"},
	}
	if err := k8sClient.Create(context.Background(), cmB); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	// A ConfigMap without the matching label - proves the label selector
	// actually scopes the informer rather than watching everything.
	cmUnlabeled := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "arasaka"},
		Data:       map[string]string{"SHOULD_NOT_APPEAR": "true"},
	}
	if err := k8sClient.Create(context.Background(), cmUnlabeled); err != nil {
		t.Fatalf("create unlabeled configmap: %v", err)
	}

	t.Run("project initial fixtures into cache", func(t *testing.T) {
		eventually(t, 10*time.Second, func() bool {
			e := appCache.Get("arasaka")
			if e == nil {
				return false
			}

			merged := e.Merged()
			return merged["LOG_LEVEL"] == "info" && merged["DARK_MODE"] == "true" && merged["SHOULD_NOT_APPEAR"] == nil
		}, "watcher did not project fixtures into cache within timeout")
	})

	t.Run("reflects ConfigMap update", func(t *testing.T) {
		patch := client.MergeFrom(cmA.DeepCopy())
		cmA.Data["LOG_LEVEL"] = "debug"
		if err := k8sClient.Patch(context.Background(), cmA, patch); err != nil {
			t.Fatalf("patch configmap: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			e := appCache.Get("arasaka")
			return e != nil && e.Merged()["LOG_LEVEL"] == "debug"
		}, "cache did not reflect ConfigMap update within timeout")
	})

	t.Run("ConfigMap delete clears only its own section (negative path)", func(t *testing.T) {
		if err := k8sClient.Delete(context.Background(), cmA); err != nil {
			t.Fatalf("delete configmap: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			e := appCache.Get("arasaka")
			if e == nil {
				return false
			}
			_, hasA := e.Sources["runtime-config"]
			return !hasA && e.Sources["feature-flags"]["DARK_MODE"] == "true"
		}, "ConfigMap delete should clear only its own source, leaving the rest of the entry intact")
	})

	t.Run("removes entry once every source is gone", func(t *testing.T) {
		if err := k8sClient.Delete(context.Background(), cmB); err != nil {
			t.Fatalf("delete configmap: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			return appCache.Get("arasaka") == nil
		}, "cache entry not removed after every source was deleted")
	})
}

// eventually polls condition every 100ms until it return true or timeout elapses.
func eventually(t *testing.T, timeout time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(msg)
}

func isMissingBinariesError(err error) bool {
	if err == nil {
		return false
	}

	s := err.Error()
	return strings.Contains(s, "executable file not found") || strings.Contains(s, "failed to start the controlplane")
}
