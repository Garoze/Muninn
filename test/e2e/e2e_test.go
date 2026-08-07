package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/kube"
)

const (
	deployNamespace = "muninn-system"
	localPort       = 15010 // distinct from `make run`'s default port
	podPort         = 5010
	e2eNamespace    = "chiba"
)

var deployAppLabels = client.MatchingLabels{"app": "muninn"}
var webhookAppLabels = client.MatchingLabels{"app": "muninn-webhook"}

const injectPodName = "netrunner"

// TestE2E deploys Muninn in-cluster via `make deploy` and exercises it
// through the real gRPC wire protocol over a port-forward. Requires a real
// cluster with the image already built and loaded (`make image load`) -
// not done here, since `make load` needs interactive sudo.
func TestE2E(t *testing.T) {
	if os.Getenv("MUNINN_IT_E2E") != "1" {
		t.Skip("set MUNINN_IT_E2E=1 to run e2e tests against a real cluster")
	}

	repoRoot := repoRoot(t)
	cfg, err := loadKubeConfig()
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}

	scheme, err := kube.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	// Self-contained fixtures in a distinct namespace, so this doesn't
	// collide with sample data a human may have already applied by hand.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: e2eNamespace}}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "runtime-config",
			Namespace: e2eNamespace,
			Labels:    map[string]string{"muninn.io/config": "runtime"},
		},
		Data: map[string]string{
			"LOG_LEVEL":    "info",
			"DISPLAY_NAME": "E2E Test Namespace",
		},
	}
	if err := k8sClient.Create(context.Background(), cm); err != nil {
		t.Fatalf("create configmap: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), cm)
	})

	runMake(t, repoRoot, "deploy")
	t.Cleanup(func() {
		runMake(t, repoRoot, "undeploy")
	})

	podName := waitForPodReady(t, k8sClient, deployNamespace, 90*time.Second, deployAppLabels)
	if podName == "" {
		checkNotImagePullError(t, k8sClient, deployNamespace)
		t.Fatal("pod never reached Ready within timeout")
	}

	stopCh, readyCh := make(chan struct{}), make(chan struct{})
	pfErrCh := startPortForward(t, cfg, deployNamespace, podName, localPort, podPort, stopCh, readyCh)
	t.Cleanup(func() { close(stopCh) })

	select {
	case <-readyCh:
	case err := <-pfErrCh:
		t.Fatalf("port-forward failed: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("port-forward never became ready")
	}

	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", localPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	discoveryClient := discoveryv1.NewDiscoveryServiceClient(conn)

	eventually(t, 20*time.Second, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := discoveryClient.Query(ctx, &discoveryv1.QueryRequest{
			Namespace: e2eNamespace,
			Keys:      []string{"LOG_LEVEL", "DISPLAY_NAME"},
		})
		if err != nil {
			return false
		}

		values := map[string]any{}
		for _, kv := range resp.GetValues() {
			values[kv.GetKey()] = kv.GetValue().AsInterface()
		}

		return values["LOG_LEVEL"] == "info" && values["DISPLAY_NAME"] == "E2E Test Namespace"
	}, "deployed Muninn did not return expected data for the e2e test namespace")

	t.Run("Describe lists the active config sources", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := discoveryClient.Describe(ctx, &discoveryv1.DescribeRequest{})
		if err != nil {
			t.Fatalf("describe: %v", err)
		}
		if len(resp.GetSources()) == 0 {
			t.Fatal("expected at least one config source")
		}
	})

	t.Run("mutating webhook injects a working config file with drift reload", func(t *testing.T) {
		if !certManagerInstalled(k8sClient) {
			t.Skip("cert-manager not installed - required by config/webhook/issuer.yaml, see README Prerequisites")
		}

		clientset, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			t.Fatalf("new clientset: %v", err)
		}

		runMake(t, repoRoot, "deploy-webhook")
		t.Cleanup(func() { runMake(t, repoRoot, "undeploy-webhook") })

		// Bound the blast radius before anything can trigger the webhook -
		// failurePolicy: Fail would otherwise block Pod admission cluster-wide.
		scopeWebhookToNamespace(t, k8sClient, e2eNamespace)

		if podName := waitForPodReady(t, k8sClient, deployNamespace, 60*time.Second, webhookAppLabels); podName == "" {
			t.Fatal("muninn-webhook pod never reached Ready - check cert-manager issued muninn-webhook-tls in time")
		}

		injectPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        injectPodName,
				Namespace:   e2eNamespace,
				Labels:      map[string]string{"app": injectPodName},
				Annotations: map[string]string{"muninn.io/inject": "true"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "app",
						Image:   "busybox:1.36",
						Command: []string{"sleep", "3600"},
					},
				},
			},
		}
		if err := k8sClient.Create(context.Background(), injectPod); err != nil {
			t.Fatalf("create annotated pod: %v", err)
		}
		t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), injectPod) })

		// 120s: busybox is pulled from a real registry, unlike this suite's
		// other pre-loaded images.
		if podName := waitForPodReady(t, k8sClient, e2eNamespace, 120*time.Second, client.MatchingLabels{"app": injectPodName}); podName == "" {
			t.Fatal("annotated pod never reached Ready - webhook injection may not have run, or busybox:1.36 pull timed out")
		}

		eventually(t, 30*time.Second, func() bool {
			out, err := execInPod(cfg, clientset, e2eNamespace, injectPodName, "app", []string{"cat", "/etc/muninn/config.yaml"})
			return err == nil && strings.Contains(out, "LOG_LEVEL: info") && strings.Contains(out, "DISPLAY_NAME: E2E Test Namespace")
		}, "injected config file never appeared with the expected resolved values")

		// Drift: edit the source ConfigMap and confirm the sidecar rewrites
		// the file in place, with no Pod restart.
		patch := client.MergeFrom(cm.DeepCopy())
		cm.Data["DISPLAY_NAME"] = "E2E Test Namespace Updated"
		if err := k8sClient.Patch(context.Background(), cm, patch); err != nil {
			t.Fatalf("patch configmap for drift check: %v", err)
		}

		eventually(t, 30*time.Second, func() bool {
			out, err := execInPod(cfg, clientset, e2eNamespace, injectPodName, "app", []string{"cat", "/etc/muninn/config.yaml"})
			return err == nil && strings.Contains(out, "DISPLAY_NAME: E2E Test Namespace Updated")
		}, "sidecar did not rewrite the config file after ConfigMap drift")
	})
}

// repoRoot resolves the project root regardless of the test binary's cwd.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// loadKubeConfig resolves the kubeconfig the same way kubectl does.
func loadKubeConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

// runMake runs `make <target>` from the project root, failing the test with
// the command's combined output on error.
func runMake(t *testing.T, repoRoot, target string) {
	t.Helper()
	cmd := exec.Command("make", target)
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("make %s failed: %v\n%s", target, err, out.String())
	}
}

// waitForPodReady polls for a Pod matching labels in namespace to reach
// Ready, returning its name. Returns "" on timeout.
func waitForPodReady(t *testing.T, k8sClient client.Client, namespace string, timeout time.Duration, labels client.MatchingLabels) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var pods corev1.PodList
		if err := k8sClient.List(context.Background(), &pods,
			client.InNamespace(namespace),
			labels,
		); err == nil {
			for _, p := range pods.Items {
				for _, cond := range p.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						return p.Name
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return ""
}

// certManagerInstalled reports whether the cert-manager namespace exists -
// an external prerequisite this repo doesn't install (see README).
func certManagerInstalled(k8sClient client.Client) bool {
	var ns corev1.Namespace
	err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "cert-manager"}, &ns)
	return err == nil
}

// scopeWebhookToNamespace patches the MutatingWebhookConfiguration to only
// intercept Pod creates in namespace. Retries on conflict because
// cert-manager's cainjector concurrently patches the same object to
// populate the CA bundle.
func scopeWebhookToNamespace(t *testing.T, k8sClient client.Client, namespace string) {
	t.Helper()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var webhookCfg admissionregistrationv1.MutatingWebhookConfiguration
		if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "muninn-webhook"}, &webhookCfg); err != nil {
			return err
		}

		for i := range webhookCfg.Webhooks {
			webhookCfg.Webhooks[i].NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
			}
		}

		return k8sClient.Update(context.Background(), &webhookCfg)
	})
	if err != nil {
		t.Fatalf("scope MutatingWebhookConfiguration to %s: %v", namespace, err)
	}
}

// execInPod runs cmd in container of the Pod named podName, returning
// combined stdout.
func execInPod(cfg *rest.Config, clientset *kubernetes.Clientset, namespace, podName, container string, cmd []string) (string, error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
	}, k8sscheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(cfg, http.MethodPost, req.URL())
	if err != nil {
		return "", fmt.Errorf("new SPDY executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		return "", fmt.Errorf("%w: stderr=%s", err, stderr.String())
	}

	return stdout.String(), nil
}

// checkNotImagePullError skips with a clearer hint when the Pod isn't Ready
// because the image isn't loaded into the node's containerd store.
func checkNotImagePullError(t *testing.T, k8sClient client.Client, namespace string) {
	t.Helper()
	var pods corev1.PodList
	if err := k8sClient.List(context.Background(), &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{"app": "muninn"},
	); err != nil {
		return
	}
	for _, p := range pods.Items {
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "ErrImageNeverPull" {
				t.Skip("image not loaded into the node's containerd store — run `make image load` first")
			}
		}
	}
}

// startPortForward opens a port-forward to podName via client-go directly,
// so lifecycle/shutdown is reliable to manage rather than driving a
// `kubectl port-forward` subprocess.
func startPortForward(
	t *testing.T,
	cfg *rest.Config,
	namespace, podName string,
	localPort, podPort int,
	stopCh, readyCh chan struct{},
) <-chan error {
	t.Helper()

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("new clientset: %v", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		t.Fatalf("spdy round tripper: %v", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	pf, err := portforward.New(
		dialer,
		[]string{fmt.Sprintf("%d:%d", localPort, podPort)},
		stopCh, readyCh,
		io.Discard, io.Discard,
	)
	if err != nil {
		t.Fatalf("new port forwarder: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- pf.ForwardPorts()
	}()
	return errCh
}

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
