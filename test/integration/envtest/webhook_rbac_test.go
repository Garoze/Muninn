package envtest_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
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
// Manifests are read directly from config/webhook/ rather than
// reconstructed in Go, so this test exercises what actually gets deployed,
// not a copy that can drift from it.

func webhookManifestPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "config", "webhook", name)
}

func applyManifest(t *testing.T, ctx context.Context, c client.Client, name string, obj client.Object) {
	t.Helper()
	data, err := os.ReadFile(webhookManifestPath(t, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := yaml.Unmarshal(data, obj); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	if err := c.Create(ctx, obj); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
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

	applyManifest(t, ctx, adminClient, "service_account.yaml", &corev1.ServiceAccount{})
	applyManifest(t, ctx, adminClient, "role.yaml", &rbacv1.ClusterRole{})
	applyManifest(t, ctx, adminClient, "role_binding.yaml", &rbacv1.ClusterRoleBinding{})
	applyManifest(t, ctx, adminClient, "role_spc_writer.yaml", &rbacv1.ClusterRole{})
	applyManifest(t, ctx, adminClient, "role_spc_writer_binding.yaml", &rbacv1.ClusterRoleBinding{})

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

	// Deliberately omit role_spc_writer.yaml/role_spc_writer_binding.yaml -
	// this is the exact configuration that denied admission against a real
	// kind cluster before those manifests existed.
	applyManifest(t, ctx, adminClient, "service_account.yaml", &corev1.ServiceAccount{})
	applyManifest(t, ctx, adminClient, "role.yaml", &rbacv1.ClusterRole{})
	applyManifest(t, ctx, adminClient, "role_binding.yaml", &rbacv1.ClusterRoleBinding{})

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

	applyManifest(t, ctx, adminClient, "service_account.yaml", &corev1.ServiceAccount{})
	applyManifest(t, ctx, adminClient, "role.yaml", &rbacv1.ClusterRole{})
	applyManifest(t, ctx, adminClient, "role_binding.yaml", &rbacv1.ClusterRoleBinding{})
	// No writer role - proves Reference mode never needs it.

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
