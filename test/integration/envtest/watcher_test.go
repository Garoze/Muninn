package envtest_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/garoze/muninn/api/v1alpha1"
	"github.com/garoze/muninn/internal/app"
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

	env := &envtest.Environment{
		CRDDirectoryPaths: []string{crdDir(t)},
	}

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

	w, err := kube.NewWatcher(cfg, scheme, appCache, metrics, nil, nil, log)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startCancel()

	if err := w.Start(startCtx); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	t.Cleanup(w.Stop)

	// Create namespace and fixtures
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-arasaka"}}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "arasaka"},
		Spec: v1alpha1.TenantSpec{
			TenantID:    "arasaka",
			DisplayName: "Arasaka Corp",
		},
	}
	if err := k8sClient.Create(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	tc := &v1alpha1.TenantConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "arasaka", Namespace: "tenant-arasaka"},
		Spec: v1alpha1.TenantConfigSpec{
			RuntimeConfig: map[string]string{"LOG_LEVEL": "info"},
		},
	}
	if err := k8sClient.Create(context.Background(), tc); err != nil {
		t.Fatalf("create tenantconfig: %v", err)
	}

	policy := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "tenant-arasaka"},
		Spec: v1alpha1.PolicySpec{
			JWT: v1alpha1.JWTConfig{
				IssuerAllowList: []string{"https://issuer.example"},
				SubjectClaim:    "sub",
				ScopesClaim:     "scp",
			},
		},
	}
	if err := k8sClient.Create(context.Background(), policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	t.Run("project initial fixtures into cache", func(t *testing.T) {
		eventually(t, 10*time.Second, func() bool {
			s := appCache.Get("arasaka")
			if s == nil {
				return false
			}

			if s.DisplayName != "Arasaka Corp" {
				return false
			}

			if s.RuntimeConfig["LOG_LEVEL"] != "info" {
				return false
			}

			if s.AuthzPolicy == nil || s.AuthzPolicy.SubjectClaim != "sub" {
				return false
			}

			return true
		}, "watcher did not project fixtures into cache within timeout")
	})

	t.Run("reflects TenantConfig update", func(t *testing.T) {
		patch := client.MergeFrom(tc.DeepCopy())
		tc.Spec.RuntimeConfig["LOG_LEVEL"] = "debug"
		if err := k8sClient.Patch(context.Background(), tc, patch); err != nil {
			t.Fatalf("patch tenantconfig: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			s := appCache.Get("arasaka")
			return s != nil && s.RuntimeConfig["LOG_LEVEL"] == "debug"
		}, "cache did not reflect TenantConfig update within timeout")
	})

	// A second, independent tenant isolates the "delete a sub-resource, Tenant
	// stays" negative path below from the "Tenant deleted, sub-resources are
	// still around (stale/orphaned)" happy path above — deleting arasaka's
	// TenantConfig/Policy here would remove the very staleness the final
	// subtest needs arasaka's fixtures to still have.
	nsYorinobu := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-yorinobu"}}
	if err := k8sClient.Create(context.Background(), nsYorinobu); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	tenantYorinobu := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "yorinobu"},
		Spec: v1alpha1.TenantSpec{
			TenantID:    "yorinobu",
			DisplayName: "Yorinobu Holdings",
		},
	}
	if err := k8sClient.Create(context.Background(), tenantYorinobu); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	tcYorinobu := &v1alpha1.TenantConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "yorinobu", Namespace: "tenant-yorinobu"},
		Spec: v1alpha1.TenantConfigSpec{
			RuntimeConfig: map[string]string{"LOG_LEVEL": "info"},
		},
	}
	if err := k8sClient.Create(context.Background(), tcYorinobu); err != nil {
		t.Fatalf("create tenantconfig: %v", err)
	}

	policyYorinobu := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "tenant-yorinobu"},
		Spec: v1alpha1.PolicySpec{
			JWT: v1alpha1.JWTConfig{
				IssuerAllowList: []string{"https://issuer.example"},
				SubjectClaim:    "sub",
				ScopesClaim:     "scp",
			},
		},
	}
	if err := k8sClient.Create(context.Background(), policyYorinobu); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	eventually(t, 10*time.Second, func() bool {
		s := appCache.Get("yorinobu")
		return s != nil && s.DisplayName == "Yorinobu Holdings" &&
			s.RuntimeConfig["LOG_LEVEL"] == "info" &&
			s.AuthzPolicy != nil && s.AuthzPolicy.SubjectClaim == "sub"
	}, "watcher did not project yorinobu fixtures into cache within timeout")

	t.Run("TenantConfig delete clears only its own section (negative path)", func(t *testing.T) {
		if err := k8sClient.Delete(context.Background(), tcYorinobu); err != nil {
			t.Fatalf("delete tenantconfig: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			s := appCache.Get("yorinobu")
			return s != nil &&
				len(s.RuntimeConfig) == 0 &&
				s.DisplayName == "Yorinobu Holdings" &&
				s.AuthzPolicy != nil && s.AuthzPolicy.SubjectClaim == "sub"
		}, "TenantConfig delete should clear only RuntimeConfig, leaving the rest of the entry intact")
	})

	t.Run("Policy delete clears only its own section (negative path)", func(t *testing.T) {
		if err := k8sClient.Delete(context.Background(), policyYorinobu); err != nil {
			t.Fatalf("delete policy: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			s := appCache.Get("yorinobu")
			return s != nil &&
				s.AuthzPolicy == nil &&
				s.DisplayName == "Yorinobu Holdings"
		}, "Policy delete should clear only AuthzPolicy, leaving the rest of the entry intact")
	})

	t.Run("removes entry on Tenant delete despite stale TenantConfig/Policy (happy path)", func(t *testing.T) {
		// arasaka's TenantConfig and Policy are deliberately never deleted —
		// they're the "stale"/orphaned records this subtest exists to prove
		// don't keep the entry alive once the Tenant itself is gone.
		if err := k8sClient.Delete(context.Background(), tenant); err != nil {
			t.Fatalf("delete tenant: %v", err)
		}

		eventually(t, 10*time.Second, func() bool {
			return appCache.Get("arasaka") == nil
		}, "cache entry not removed after Tenant delete within timeout")

		var staleTC v1alpha1.TenantConfig
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(tc), &staleTC); err != nil {
			t.Fatalf("expected arasaka's TenantConfig to still exist (orphaned, not cascade-deleted): %v", err)
		}

		var stalePolicy v1alpha1.Policy
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(policy), &stalePolicy); err != nil {
			t.Fatalf("expected arasaka's Policy to still exist (orphaned, not cascade-deleted): %v", err)
		}
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

// crdDir resolves the project-root config/crd directory relative to this file.
func crdDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// test/integration/envtest/ -> ../../.. -> project root
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "config", "crd"))
}

func isMissingBinariesError(err error) bool {
	if err == nil {
		return false
	}

	s := err.Error()
	return strings.Contains(s, "executable file not found") || strings.Contains(s, "failed to start the controlplane")
}
