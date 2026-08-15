package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/garoze/muninn/internal/kube"
)

const csiClusterName = "muninn-csi-e2e-test"

// TestCSIE2E is the automated form of the CSI secret-delivery runbook,
// exercising the full pipeline against a real, disposable kind cluster
// this test provisions itself: a webhook-generated (not hand-written)
// SecretProviderClass, config and a real Vault secret landing in the same
// Pod simultaneously. Not TestE2E's job - that test assumes an existing
// cluster and never touches CSI/SecretProviderClass at all. Heavy
// (multi-minute) and requires kind/podman/helm/kubectl, so it's gated
// separately from MUNINN_IT_E2E and never runs in CI, matching TestE2E's
// own local-only precedent.
func TestCSIE2E(t *testing.T) {
	if os.Getenv("MUNINN_IT_CSI_E2E") != "1" {
		t.Skip("set MUNINN_IT_CSI_E2E=1 to run the CSI secret-delivery e2e test")
	}
	for _, bin := range []string{"kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found in PATH, required for this test", bin)
		}
	}

	repoRoot := repoRoot(t)
	kubeconfigPath := t.TempDir() + "/kubeconfig"

	runCmd(t, "", nil, "kind", "create", "cluster", "--name", csiClusterName, "--kubeconfig", kubeconfigPath)
	t.Cleanup(func() {
		_ = exec.Command("kind", "delete", "cluster", "--name", csiClusterName).Run()
	})

	env := append(os.Environ(), "KUBECONFIG="+kubeconfigPath)

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		t.Fatalf("build rest.Config from kind kubeconfig: %v", err)
	}
	scheme, err := kube.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}
	k8sClient, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("new clientset: %v", err)
	}

	// --- secrets-store-csi-driver ---
	runCmd(t, "", env, "helm", "repo", "add", "secrets-store-csi-driver", "https://kubernetes-sigs.github.io/secrets-store-csi-driver/charts")
	runCmd(t, "", env, "helm", "repo", "add", "hashicorp", "https://helm.releases.hashicorp.com")
	runCmd(t, "", env, "helm", "repo", "update")
	runCmd(t, "", env, "helm", "install", "csi-driver", "secrets-store-csi-driver/secrets-store-csi-driver",
		"--namespace", "kube-system", "--set", "syncSecret.enabled=false")

	// --- Vault (dev mode) + its CSI provider ---
	runCmd(t, "", env, "helm", "install", "vault", "hashicorp/vault",
		"--namespace", "kube-system", "--set", "server.dev.enabled=true", "--set", "csi.enabled=true")

	if podName := waitForPodReady(t, k8sClient, "kube-system", 120*time.Second, client.MatchingLabels{"app": "secrets-store-csi-driver"}); podName == "" {
		t.Fatal("secrets-store-csi-driver pod never reached Ready")
	}
	if podName := waitForPodReady(t, k8sClient, "kube-system", 120*time.Second, client.MatchingLabels{"app.kubernetes.io/name": "vault"}); podName == "" {
		t.Fatal("vault-0 pod never reached Ready")
	}
	if podName := waitForPodReady(t, k8sClient, "kube-system", 120*time.Second, client.MatchingLabels{"app.kubernetes.io/name": "vault-csi-provider"}); podName == "" {
		t.Fatal("vault-csi-provider pod never reached Ready")
	}

	// --- configure Vault: secret + Kubernetes auth ---
	vaultScript := `
  vault kv put secret/arasaka/db-password value=hunter2

  vault auth enable kubernetes
  vault write auth/kubernetes/config \
    kubernetes_host="https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT"

  vault policy write muninn-read - <<'EOF'
path "secret/data/arasaka/db-password" {
  capabilities = ["read"]
}
EOF

  vault write auth/kubernetes/role/muninn \
    bound_service_account_names=default \
    bound_service_account_namespaces=arasaka \
    policies=muninn-read \
    ttl=15m
`
	if out, err := execInPod(restCfg, clientset, "kube-system", "vault-0", "vault", []string{"sh", "-c", vaultScript}); err != nil {
		t.Fatalf("configure vault: %v\n%s", err, out)
	}

	// --- build and load Muninn's image ---
	runMake(t, repoRoot, "image", "IMG="+localImage)
	loadImageIntoKind(t, repoRoot, csiClusterName)

	// --- cert-manager, then serve + webhook, pointed at this Vault ---
	runCmd(t, "", env, "kubectl", "apply", "-f", "https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml")
	waitAllPodsReady(t, k8sClient, "cert-manager", 120*time.Second)

	runCmd(t, repoRoot, env, "make", "deploy", "IMG="+localImage)
	t.Cleanup(func() { runCmd(t, repoRoot, env, "make", "undeploy") })
	if podName := waitForPodReady(t, k8sClient, deployNamespace, 90*time.Second, deployAppLabels); podName == "" {
		t.Fatal("muninn (serve) pod never reached Ready")
	}

	runCmd(t, repoRoot, env, "make", "deploy-webhook", "IMG="+localImage)
	t.Cleanup(func() { runCmd(t, repoRoot, env, "make", "undeploy-webhook") })
	if podName := waitForPodReady(t, k8sClient, deployNamespace, 90*time.Second, webhookAppLabels); podName == "" {
		t.Fatal("muninn-webhook pod never reached Ready")
	}

	// --- consumer namespace, ConfigMap, and test Pod ---
	if err := k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "arasaka"}}); err != nil {
		t.Fatalf("create arasaka namespace: %v", err)
	}

	// Grants the sidecar's own ServiceAccount (the consumer Pod's, not
	// muninn-webhook's) events.k8s.io create - proves the drift Event path
	// for real, on a namespace that opted in, without muninn-webhook ever
	// granting it. Applies the actual config/samples/ files (the same ones
	// `make sample-events` applies) rather than reconstructing the same
	// RBAC in Go, so this test exercises what a user would actually run.
	runCmd(t, "", env, "kubectl", "apply", "-f", filepath.Join(repoRoot, "config/samples/event_writer_role.yaml"))
	runCmd(t, "", env, "kubectl", "apply", "-f", filepath.Join(repoRoot, "config/samples/event_writer_role_binding.yaml"))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "arasaka-config",
			Namespace: "arasaka",
			Labels:    map[string]string{"muninn.io/config": "runtime"},
		},
		Data: map[string]string{
			"log_level":        "debug",
			"db_password_ref":  "vault://secret/data/arasaka/db-password",
			"db_password_key":  "value",
			"db_password_file": "/mnt/secrets-store/db_password",
		},
	}
	if err := k8sClient.Create(context.Background(), cm); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "netrunner",
			Namespace:   "arasaka",
			Labels:      map[string]string{"app": "netrunner"},
			Annotations: map[string]string{"muninn.io/inject": "true"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "default",
			Containers: []corev1.Container{
				{Name: "app", Image: "busybox:1.36", Command: []string{"sleep", "3600"}},
			},
		},
	}
	// Pod Ready doesn't guarantee the webhook's Service endpoint has
	// propagated through kube-proxy yet, or that ListenAndServeTLS has
	// finished binding - a real, observed transient race on a freshly
	// created cluster, not a webhook bug. Retry only on that specific
	// failure mode.
	var createErr error
	for range 15 {
		createErr = k8sClient.Create(context.Background(), pod)
		if createErr == nil {
			break
		}
		if !strings.Contains(createErr.Error(), "connection refused") && !strings.Contains(createErr.Error(), "failed calling webhook") {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if createErr != nil {
		t.Fatalf("create test pod: %v", createErr)
	}

	// 120s: busybox pulled from a real registry, and the CSI mount adds a
	// dependency chain (webhook -> reconcile SPC -> driver -> Vault) beyond
	// TestE2E's plain config-only injection.
	if podName := waitForPodReady(t, k8sClient, "arasaka", 120*time.Second, client.MatchingLabels{"app": "netrunner"}); podName == "" {
		t.Fatal("netrunner pod never reached Ready - webhook injection, SPC reconcile, or the CSI mount may have failed")
	}

	// --- the actual proof ---
	t.Run("webhook generated the SecretProviderClass, not hand-written", func(t *testing.T) {
		var spc secretsstorev1.SecretProviderClass
		if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "arasaka", Name: "muninn-secrets-arasaka"}, &spc); err != nil {
			t.Fatalf("get webhook-generated SecretProviderClass: %v", err)
		}
		if spc.Spec.Provider != "vault" {
			t.Errorf("SecretProviderClass provider: got %q, want vault", spc.Spec.Provider)
		}
		if !strings.Contains(spc.Spec.Parameters["objects"], "objectName: db_password") {
			t.Errorf("SecretProviderClass objects: got %q, missing objectName: db_password", spc.Spec.Parameters["objects"])
		}
	})

	t.Run("config file delivered correctly", func(t *testing.T) {
		out, err := execInPod(restCfg, clientset, "arasaka", "netrunner", "app", []string{"cat", "/etc/muninn/config.yaml"})
		if err != nil {
			t.Fatalf("cat config.yaml: %v\n%s", err, out)
		}
		if !strings.Contains(out, "log_level: debug") {
			t.Errorf("config file missing expected content: %s", out)
		}
	})

	t.Run("secret file delivered correctly by the CSI driver, not Muninn", func(t *testing.T) {
		out, err := execInPod(restCfg, clientset, "arasaka", "netrunner", "app", []string{"cat", "/mnt/secrets-store/db_password"})
		if err != nil {
			t.Fatalf("cat secret file: %v\n%s", err, out)
		}
		if strings.TrimSpace(out) != "hunter2" {
			t.Errorf("secret file content: got %q, want %q", strings.TrimSpace(out), "hunter2")
		}
	})

	t.Run("sidecar reports drift when a new secret reference appears", func(t *testing.T) {
		// The sidecar polls on sidecarWatchInterval (15s, internal/webhook/inject.go)
		// - adding a new *_ref key after the Pod is already running is the
		// real trigger for this path, not something reachable at admission
		// time.
		patch := client.MergeFrom(cm.DeepCopy())
		cm.Data["api_key_ref"] = "vault://secret/data/arasaka/api-key"
		if err := k8sClient.Patch(context.Background(), cm, patch); err != nil {
			t.Fatalf("patch configmap to add a new secret reference: %v", err)
		}

		eventually(t, 40*time.Second, func() bool {
			out, err := clientset.CoreV1().Pods("arasaka").GetLogs("netrunner", &corev1.PodLogOptions{
				Container: "muninn-resolve-sidecar",
			}).DoRaw(context.Background())
			return err == nil && strings.Contains(string(out), "drift: new secret reference(s) api_key_ref appeared")
		}, "sidecar logs never showed the drift line for the newly-appeared api_key_ref")

		eventually(t, 15*time.Second, func() bool {
			var events eventsv1.EventList
			if err := k8sClient.List(context.Background(), &events, client.InNamespace("arasaka")); err != nil {
				return false
			}
			for _, ev := range events.Items {
				if ev.Reason == "NewSecretReference" && strings.Contains(ev.Note, "api_key_ref") {
					return true
				}
			}
			return false
		}, "no NewSecretReference Event appeared - the event-writer Role/RoleBinding may not have taken effect")
	})
}

// runCmd runs name/args, failing the test with combined output on error.
// dir defaults to the current directory when empty; env defaults to the
// current process environment when nil.
func runCmd(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out.String())
	}
}

// loadImageIntoKind loads the tarball `make image` already produced
// (bin/image.tar - ko's own output, not a container-engine save) into
// clusterName via kind's image-archive loader.
func loadImageIntoKind(t *testing.T, repoRoot, clusterName string) {
	t.Helper()
	tarPath := filepath.Join(repoRoot, "bin", "image.tar")
	runCmd(t, "", nil, "kind", "load", "image-archive", tarPath, "--name", clusterName)
}

// waitAllPodsReady fails the test if not every Pod in namespace reaches
// Ready within timeout - unlike waitForPodReady, which only needs one
// matching Pod, this is for prerequisites (cert-manager) where a partial
// rollout isn't good enough.
func waitAllPodsReady(t *testing.T, k8sClient client.Client, namespace string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var pods corev1.PodList
		if err := k8sClient.List(context.Background(), &pods, client.InNamespace(namespace)); err == nil && len(pods.Items) > 0 {
			allReady := true
			for _, p := range pods.Items {
				ready := false
				for _, cond := range p.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						ready = true
					}
				}
				if !ready {
					allReady = false
					break
				}
			}
			if allReady {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("not all pods in %s became Ready within %s", namespace, timeout)
}
