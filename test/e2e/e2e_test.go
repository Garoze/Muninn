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
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/garoze/muninn/api/v1alpha1"
	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/kube"
)

const (
	deployNamespace = "muninn-system"
	deployAppLabel  = "app=muninn"
	localPort       = 15010 // avoid colliding with a locally-running `make run`
	podPort         = 5010
	e2eTenantID     = "e2e-tenant"
)

// TestE2E deploys Muninn in-cluster via the real `make deploy` target,
// exercises it through the actual gRPC wire protocol over a port-forward,
// and tears down via `make undeploy`. Requires a real cluster and the image
// already built and loaded (`make image load`) — this test does not do
// either itself, since `make load` needs interactive sudo.
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

	// Self-contained fixtures with a distinct tenant ID, so this doesn't
	// collide with sample data a human may have already applied by hand.
	tenantNamespace := "tenant-" + e2eTenantID
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenantNamespace}}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: e2eTenantID},
		Spec: v1alpha1.TenantSpec{
			TenantID:    e2eTenantID,
			DisplayName: "E2E Test Tenant",
		},
	}
	if err := k8sClient.Create(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), tenant)
	})

	runMake(t, repoRoot, "deploy")
	t.Cleanup(func() {
		runMake(t, repoRoot, "undeploy")
	})

	podName := waitForPodReady(t, k8sClient, deployNamespace, 90*time.Second)
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
			TenantId: e2eTenantID,
			Keys:     []string{"TENANT.id", "TENANT.displayName"},
		})
		if err != nil {
			return false
		}

		values := map[string]any{}
		for _, kv := range resp.GetValues() {
			values[kv.GetKey()] = kv.GetValue().AsInterface()
		}

		return values["TENANT.id"] == e2eTenantID && values["TENANT.displayName"] == "E2E Test Tenant"
	}, "deployed Muninn did not return expected data for the e2e test tenant")

	t.Run("Describe lists the supported key namespace", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := discoveryClient.Describe(ctx, &discoveryv1.DescribeRequest{})
		if err != nil {
			t.Fatalf("describe: %v", err)
		}
		if len(resp.GetSupportedKeys()) == 0 {
			t.Fatal("expected at least one supported key")
		}
	})
}

// repoRoot resolves the project root relative to this file
// (test/e2e/ -> ../.. -> project root), so `make` runs with the right cwd
// regardless of the test binary's own working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// loadKubeConfig resolves the kubeconfig the same way kubectl does: $KUBECONFIG
// (colon-separated on non-Windows), falling back to ~/.kube/config.
func loadKubeConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

// runMake runs `make <target>` with cwd set to the project root, failing the
// test with the command's combined output on error.
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

// waitForPodReady polls for a Pod matching deployAppLabel in namespace to
// reach Ready, returning its name. Returns "" on timeout.
func waitForPodReady(t *testing.T, k8sClient client.Client, namespace string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var pods corev1.PodList
		if err := k8sClient.List(context.Background(), &pods,
			client.InNamespace(namespace),
			client.MatchingLabels{"app": "muninn"},
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

// checkNotImagePullError logs a clearer skip/failure hint when the Pod never
// became Ready because the image isn't loaded into the node's containerd
// store — the one prerequisite this test deliberately doesn't set up itself.
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

// startPortForward opens a port-forward to podName using client-go directly
// (not a `kubectl port-forward` subprocess) so its lifecycle — readiness,
// shutdown on stopCh — is reliable to manage from a test rather than
// parsing another process's stdout/killing it on cleanup.
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

// eventually polls condition every 100ms until it returns true or timeout
// elapses.
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
