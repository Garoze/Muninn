package main

import (
	"context"
	"os"
	"strings"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestDriftReporter_* proves driftReporter's real RBAC-facing behavior
// against a real, RBAC-enforcing API server (envtest defaults to
// authorization-mode: RBAC) - complementary to drift_test.go's unit tests,
// which only cover "no in-cluster config at all" and "no Pod identity."
// Neither of those exercises the actual Create call's RBAC-denied path,
// which is the whole point of the "best-effort" design: a namespace that
// hasn't granted events.k8s.io create must still get the log, not an
// error or a lost signal.
//
// Lives in cmd/muninn (package main), not test/integration/envtest: Go
// doesn't allow importing package main, and driftReporter is unexported,
// so a test exercising it against a real API server has to live here.

func isMissingBinariesError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "executable file not found") || strings.Contains(s, "failed to start the controlplane")
}

// mintScopedClientset mints a real TokenRequest-backed token for
// serviceAccount in namespace and returns a *kubernetes.Clientset
// authenticated as it - matching test/integration/envtest/webhook_rbac_test.go's
// mintScopedClient, but for client-go rather than controller-runtime, since
// driftReporter.clientset is a *kubernetes.Clientset.
func mintScopedClientset(t *testing.T, ctx context.Context, restCfg *rest.Config, adminClientset *kubernetes.Clientset, namespace, serviceAccount string) *kubernetes.Clientset {
	t.Helper()

	tr, err := adminClientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, serviceAccount, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("mint token for %s/%s: %v", namespace, serviceAccount, err)
	}

	scopedCfg := rest.CopyConfig(restCfg)
	scopedCfg.BearerToken = tr.Status.Token
	scopedCfg.BearerTokenFile = ""
	scopedCfg.CertData = nil
	scopedCfg.KeyData = nil
	scopedCfg.CertFile = ""
	scopedCfg.KeyFile = ""

	cs, err := kubernetes.NewForConfig(scopedCfg)
	if err != nil {
		t.Fatalf("new scoped clientset: %v", err)
	}
	return cs
}

// startDriftEnvtest starts envtest and creates the arasaka namespace plus
// a "netrunner" ServiceAccount every test below authenticates as.
func startDriftEnvtest(t *testing.T) (*rest.Config, *kubernetes.Clientset, client.Client) {
	t.Helper()

	env := &envtest.Environment{}
	restCfg, err := env.Start()
	if err != nil {
		if isMissingBinariesError(err) {
			t.Skipf("envtest binaries not found (set KUBEBUILDER_ASSETS): %v", err)
		}
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	adminClientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("new admin clientset: %v", err)
	}
	adminClient, err := client.New(restCfg, client.Options{})
	if err != nil {
		t.Fatalf("new admin client: %v", err)
	}

	ctx := context.Background()
	if err := adminClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "arasaka"}}); err != nil {
		t.Fatalf("create arasaka namespace: %v", err)
	}
	if err := adminClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "netrunner", Namespace: "arasaka"}}); err != nil {
		t.Fatalf("create netrunner ServiceAccount: %v", err)
	}

	return restCfg, adminClientset, adminClient
}

func TestDriftReporter_WithEventsRBAC_CreatesRealEvent(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}
	ctx := context.Background()
	restCfg, adminClientset, adminClient := startDriftEnvtest(t)

	if err := adminClient.Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "event-writer", Namespace: "arasaka"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"events.k8s.io"}, Resources: []string{"events"}, Verbs: []string{"create"}},
		},
	}); err != nil {
		t.Fatalf("create Role: %v", err)
	}
	if err := adminClient.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "event-writer", Namespace: "arasaka"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "event-writer"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "netrunner", Namespace: "arasaka"}},
	}); err != nil {
		t.Fatalf("create RoleBinding: %v", err)
	}

	scopedClientset := mintScopedClientset(t, ctx, restCfg, adminClientset, "arasaka", "netrunner")

	r := &driftReporter{podName: "netrunner-pod", podNamespace: "arasaka"}
	r.once.Do(func() {}) // skip lazy in-cluster init - clientset is injected directly
	r.clientset = scopedClientset

	stderr := captureStderr(t)
	r.Report(ctx, "arasaka", []string{"api_key_ref"})
	if out := stderr(); !strings.Contains(out, "drift: new secret reference(s) api_key_ref appeared") {
		t.Errorf("expected the log line regardless of Event success, got: %s", out)
	}

	var events eventsv1.EventList
	if err := adminClient.List(ctx, &events, client.InNamespace("arasaka")); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events.Items), events.Items)
	}
	ev := events.Items[0]
	if ev.Reason != "NewSecretReference" {
		t.Errorf("Reason: got %q, want NewSecretReference", ev.Reason)
	}
	if ev.Regarding.Kind != "Pod" || ev.Regarding.Name != "netrunner-pod" || ev.Regarding.Namespace != "arasaka" {
		t.Errorf("Regarding: got %+v", ev.Regarding)
	}
	if !strings.Contains(ev.Note, "api_key_ref") {
		t.Errorf("Note: got %q, want it to mention api_key_ref", ev.Note)
	}
	if ev.Type != "Warning" {
		t.Errorf("Type: got %q, want Warning", ev.Type)
	}
}

func TestDriftReporter_WithoutEventsRBAC_LogsOnlyNoEventCreated(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}
	ctx := context.Background()
	restCfg, adminClientset, adminClient := startDriftEnvtest(t)
	// Deliberately no Role/RoleBinding granting events:create - this is the
	// realistic default for a consumer namespace that never opted in.

	scopedClientset := mintScopedClientset(t, ctx, restCfg, adminClientset, "arasaka", "netrunner")

	r := &driftReporter{podName: "netrunner-pod", podNamespace: "arasaka"}
	r.once.Do(func() {})
	r.clientset = scopedClientset

	stderr := captureStderr(t)
	r.Report(ctx, "arasaka", []string{"api_key_ref"})
	out := stderr()

	if !strings.Contains(out, "drift: new secret reference(s) api_key_ref appeared") {
		t.Errorf("expected the drift log line even without Event RBAC, got: %s", out)
	}
	if !strings.Contains(out, "failed to emit drift Event") {
		t.Errorf("expected a warning about the failed Event attempt, got: %s", out)
	}

	var events eventsv1.EventList
	if err := adminClient.List(ctx, &events, client.InNamespace("arasaka")); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 0 {
		t.Errorf("expected no Event to have been created without RBAC, got %+v", events.Items)
	}
}
