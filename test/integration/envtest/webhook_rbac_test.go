package envtest_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	"github.com/garoze/muninn/internal/config"
	kubeModule "github.com/garoze/muninn/internal/kube"
	webhookModule "github.com/garoze/muninn/internal/webhook"
)

// secretsStoreCRDDir locates config/crd/bases inside the
// sigs.k8s.io/secrets-store-csi-driver module already in the local module
// cache (resolved via build info + `go env GOMODCACHE`, not a hardcoded
// path) - envtest needs the real SecretProviderClass/
// SecretProviderClassPodStatus CRD definitions installed, since unlike
// corev1.ConfigMap these are CRDs envtest's bare control plane doesn't
// know about by default.
func secretsStoreCRDDir(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "sigs.k8s.io/secrets-store-csi-driver").Output()
	if err != nil {
		t.Fatalf("go list -m sigs.k8s.io/secrets-store-csi-driver: %v", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "config", "crd", "bases")
}

// TestWebhookRBAC_* proves internal/webhook's actual RBAC needs against a
// real, RBAC-enforcing API server (envtest runs with authorization-mode:
// RBAC by default), using a client scoped to the real muninn-webhook
// ServiceAccount via a minted TokenRequest - not the admin/unrestricted
// client every other envtest test in this package uses. This is the class
// of bug a fake client (internal/webhook/secretproviderclass_test.go) can
// never catch, since RBAC isn't enforced there at all - it caught a real
// one: role_spc_writer.yaml originally granted create/update, but
// Server-Side Apply needs patch, not update; only a real API server
// denies that.
//
// RBAC is rendered from the chart rather than reconstructed in Go, so this
// test exercises what actually gets deployed, not a copy that can drift
// from it. The chart's unit tests already assert which documents each
// spcMode renders; what they cannot assert is whether those grants
// authorize anything, since nothing enforces RBAC during templating. Each
// case below therefore selects its RBAC by setting the value a user would
// set, and lets a real API server decide the outcome.

var chartDepsOnce sync.Once

// chartPath resolves the chart relative to this test file.
func chartPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "charts", "muninn")
}

// renderChartRBAC renders the chart with sets applied and returns every
// ServiceAccount, ClusterRole and ClusterRoleBinding in the output. The
// remaining kinds are dropped: the namespaces are created separately, and
// nothing else here has any bearing on what the webhook's identity is
// allowed to do.
func renderChartRBAC(t *testing.T, sets ...string) []*unstructured.Unstructured {
	t.Helper()

	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, required to render the chart's RBAC")
	}
	// Helm refuses to render a chart whose declared dependencies are missing
	// from disk even when every one of them is condition-disabled, as they
	// are here, and those archives are gitignored.
	chartDepsOnce.Do(func() {
		if out, err := exec.Command("helm", "dependency", "build", chartPath(t)).CombinedOutput(); err != nil {
			t.Fatalf("helm dependency build: %v\n%s", err, out)
		}
	})

	args := []string{"template", "muninn", chartPath(t), "--namespace", "muninn-system"}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	out, err := exec.Command("helm", args...).Output()
	if err != nil {
		t.Fatalf("helm template: %v", err)
	}

	var objs []*unstructured.Unstructured
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(out)))
	for {
		doc, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("split rendered manifests: %v", err)
		}
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(doc, obj); err != nil {
			t.Fatalf("decode rendered manifest: %v", err)
		}
		switch obj.GetKind() {
		case "ServiceAccount", "ClusterRole", "ClusterRoleBinding":
			objs = append(objs, obj)
		}
	}
	if len(objs) == 0 {
		t.Fatalf("no RBAC rendered from the chart with %v", sets)
	}
	return objs
}

func applyAll(t *testing.T, ctx context.Context, c client.Client, objs []*unstructured.Unstructured) {
	t.Helper()
	for _, obj := range objs {
		if err := c.Create(ctx, obj); err != nil {
			t.Fatalf("create %s %s: %v", obj.GetKind(), obj.GetName(), err)
		}
	}
}

// hasClusterRole reports whether objs contains a ClusterRole named name -
// used to assert the chart rendered (or withheld) the writer role, so a
// case that means to run without it fails loudly if the chart ever starts
// granting it unconditionally.
func hasClusterRole(objs []*unstructured.Unstructured, name string) bool {
	for _, obj := range objs {
		if obj.GetKind() == "ClusterRole" && obj.GetName() == name {
			return true
		}
	}
	return false
}

// mintScopedClient mints a real TokenRequest-backed token for
// serviceAccount in namespace and returns a client authenticated as it -
// not the cluster admin. envtest's default API server config includes
// service-account-issuer/service-account-signing-key-file, so TokenRequest
// works without extra setup.
func mintScopedClient(t *testing.T, ctx context.Context, restCfg *rest.Config, scheme *apiruntime.Scheme, namespace, serviceAccount string) client.Client {
	t.Helper()

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		t.Fatalf("new clientset: %v", err)
	}

	tr, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, serviceAccount, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("mint token for %s/%s: %v", namespace, serviceAccount, err)
	}

	scopedCfg := rest.CopyConfig(restCfg)
	scopedCfg.BearerToken = tr.Status.Token
	scopedCfg.BearerTokenFile = ""
	// Admin auth (envtest's default client cert) must not leak into the
	// scoped config, or every RBAC check below would silently pass as
	// cluster-admin regardless of what the ClusterRole actually grants.
	scopedCfg.CertData = nil
	scopedCfg.KeyData = nil
	scopedCfg.CertFile = ""
	scopedCfg.KeyFile = ""

	scopedClient, err := client.New(scopedCfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new scoped client: %v", err)
	}
	return scopedClient
}

// startRBACEnvtest starts envtest and creates the muninn-system/arasaka
// namespaces every test below needs, returning the admin client and rest
// config for further setup.
func startRBACEnvtest(t *testing.T) (*rest.Config, client.Client) {
	t.Helper()

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{secretsStoreCRDDir(t)},
		ErrorIfCRDPathMissing: true,
	}
	restCfg, err := env.Start()
	if err != nil {
		if isMissingBinariesError(err) {
			t.Skipf("envtest binaries not found (set KUBEBUILDER_ASSETS): %v", err)
		}
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	scheme, err := kubeModule.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}
	adminClient, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new admin client: %v", err)
	}

	ctx := context.Background()
	if err := adminClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "muninn-system"}}); err != nil {
		t.Fatalf("create muninn-system namespace: %v", err)
	}
	if err := adminClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "arasaka"}}); err != nil {
		t.Fatalf("create arasaka namespace: %v", err)
	}

	return restCfg, adminClient
}

func TestWebhookRBAC_WriterRoleApplied_CreateModeSucceeds(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}
	ctx := context.Background()
	restCfg, adminClient := startRBACEnvtest(t)

	objs := renderChartRBAC(t, "secrets.enabled=true", "secrets.spcMode=Create")
	if !hasClusterRole(objs, "muninn-webhook-spc-writer") {
		t.Fatal("Create mode rendered no writer ClusterRole")
	}
	applyAll(t, ctx, adminClient, objs)

	scheme, _ := kubeModule.NewScheme()
	scopedClient := mintScopedClient(t, ctx, restCfg, scheme, "muninn-system", "muninn-webhook")

	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeCreate, VaultAddress: "http://vault.kube-system:8200", VaultRoleName: "muninn"}
	refs := []webhookModule.SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/arasaka/db-password"}}

	if err := webhookModule.ReconcileSecretProviderClass(ctx, scopedClient, cfg, "arasaka", refs, false); err != nil {
		t.Fatalf("ReconcileSecretProviderClass with writer role applied: got %v, want success", err)
	}
}

func TestWebhookRBAC_WriterRoleNotApplied_CreateModeDenied(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}
	ctx := context.Background()
	restCfg, adminClient := startRBACEnvtest(t)

	// Reference mode renders no writer role, which is the configuration that
	// denied admission against a real kind cluster before that role existed.
	// Running Create mode against it is what a misconfigured deployment does.
	objs := renderChartRBAC(t, "secrets.enabled=true", "secrets.spcMode=Reference")
	if hasClusterRole(objs, "muninn-webhook-spc-writer") {
		t.Fatal("Reference mode rendered the writer ClusterRole, which defeats this case")
	}
	applyAll(t, ctx, adminClient, objs)

	scheme, _ := kubeModule.NewScheme()
	scopedClient := mintScopedClient(t, ctx, restCfg, scheme, "muninn-system", "muninn-webhook")

	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeCreate, VaultAddress: "http://vault.kube-system:8200", VaultRoleName: "muninn"}
	refs := []webhookModule.SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/arasaka/db-password"}}

	err := webhookModule.ReconcileSecretProviderClass(ctx, scopedClient, cfg, "arasaka", refs, false)
	if err == nil {
		t.Fatal("expected a permission error without the writer role, got nil")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected a forbidden/RBAC-denied error, got: %v", err)
	}
}

func TestWebhookRBAC_ReferenceMode_BaseRoleAloneSuffices(t *testing.T) {
	if os.Getenv("MUNINN_IT_ENVTEST") != "1" {
		t.Skip("set MUNINN_IT_ENVTEST=1 to run envtest integration tests")
	}
	ctx := context.Background()
	restCfg, adminClient := startRBACEnvtest(t)

	// No writer role - proves Reference mode never needs it.
	applyAll(t, ctx, adminClient, renderChartRBAC(t, "secrets.enabled=true", "secrets.spcMode=Reference"))

	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeReference, VaultAddress: "http://vault.kube-system:8200", VaultRoleName: "muninn"}
	refs := []webhookModule.SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/arasaka/db-password"}}

	// Pre-provision the SPC via the admin client in Create mode first,
	// simulating "this object already exists correctly" (AOT mode's own
	// contract - a platform team provisioned it out of band). Reuses the
	// real exported reconcile logic instead of reconstructing its internal
	// spec-building by hand, so the two calls build byte-identical specs.
	setupCfg := &config.Config{SecretSPCMode: config.SecretSPCModeCreate, VaultAddress: cfg.VaultAddress, VaultRoleName: cfg.VaultRoleName}
	if err := webhookModule.ReconcileSecretProviderClass(ctx, adminClient, setupCfg, "arasaka", refs, false); err != nil {
		t.Fatalf("pre-provision SecretProviderClass via admin client: %v", err)
	}

	scheme, _ := kubeModule.NewScheme()
	scopedClient := mintScopedClient(t, ctx, restCfg, scheme, "muninn-system", "muninn-webhook")

	if err := webhookModule.ReconcileSecretProviderClass(ctx, scopedClient, cfg, "arasaka", refs, false); err != nil {
		t.Fatalf("Reference mode with only the base role: got %v, want success (get-only should be enough)", err)
	}
}
