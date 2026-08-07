package webhook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/garoze/muninn/internal/config"
)

func testSPCConfig(mode config.SecretSPCMode) *config.Config {
	return &config.Config{
		SecretSPCMode: mode,
		VaultAddress:  "http://vault.kube-system:8200",
		VaultRoleName: "muninn",
	}
}

func testSPCScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := secretsstorev1.Install(s); err != nil {
		t.Fatalf("install scheme: %v", err)
	}
	return s
}

func decodeObjects(t *testing.T, spec secretsstorev1.SecretProviderClassSpec) []vaultObject {
	t.Helper()
	var objs []vaultObject
	if err := yaml.Unmarshal([]byte(spec.Parameters["objects"]), &objs); err != nil {
		t.Fatalf("decode objects param: %v", err)
	}
	return objs
}

func TestBuildSecretProviderClassSpec_SingleRef(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)
	refs := []SecretRef{
		{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"},
	}

	spec := buildSecretProviderClassSpec(cfg, refs)

	if spec.Provider != secretsstorev1.Provider("vault") {
		t.Errorf("Provider = %q, want vault", spec.Provider)
	}
	if spec.Parameters["vaultAddress"] != cfg.VaultAddress {
		t.Errorf("vaultAddress = %q, want %q", spec.Parameters["vaultAddress"], cfg.VaultAddress)
	}
	if spec.Parameters["roleName"] != cfg.VaultRoleName {
		t.Errorf("roleName = %q, want %q", spec.Parameters["roleName"], cfg.VaultRoleName)
	}

	objs := decodeObjects(t, spec)
	if len(objs) != 1 {
		t.Fatalf("got %d objects, want 1: %+v", len(objs), objs)
	}
	want := vaultObject{ObjectName: "db_password", SecretPath: "secret/data/prod/db-password", SecretKey: "value"}
	if objs[0] != want {
		t.Errorf("got %+v, want %+v", objs[0], want)
	}
}

func TestBuildSecretProviderClassSpec_NoSecretKey_Omitted(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)
	refs := []SecretRef{
		{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password"},
	}

	spec := buildSecretProviderClassSpec(cfg, refs)

	if strings.Contains(spec.Parameters["objects"], "secretKey") {
		t.Errorf("expected secretKey to be omitted from objects YAML, got %q", spec.Parameters["objects"])
	}
}

func TestBuildSecretProviderClassSpec_EmptyRefs(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)

	spec := buildSecretProviderClassSpec(cfg, nil)

	objs := decodeObjects(t, spec)
	if len(objs) != 0 {
		t.Errorf("expected no objects for empty refs, got %+v", objs)
	}
}

func TestReconcileSecretProviderClass_Create_NotFound_Creates(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"}}

	if err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := &secretsstorev1.SecretProviderClass{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "prod", Name: "muninn-secrets-prod"}, got); err != nil {
		t.Fatalf("expected SPC to be created, Get failed: %v", err)
	}
	if got.Spec.Provider != secretsstorev1.Provider("vault") {
		t.Errorf("created SPC Provider = %q, want vault", got.Spec.Provider)
	}
}

func TestReconcileSecretProviderClass_Create_ExistingMatches_NoOp(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)
	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"}}
	wantSpec := buildSecretProviderClassSpec(cfg, refs)

	existing := &secretsstorev1.SecretProviderClass{
		ObjectMeta: objMeta("muninn-secrets-prod", "prod"),
		Spec:       wantSpec,
	}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).WithObjects(existing).Build()

	if err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
}

func TestReconcileSecretProviderClass_Create_ExistingDiffers_Updates(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)
	staleRefs := []SecretRef{{Key: "old_ref", ObjectName: "old", Provider: "vault", Path: "secret/data/prod/old"}}
	existing := &secretsstorev1.SecretProviderClass{
		ObjectMeta: objMeta("muninn-secrets-prod", "prod"),
		Spec:       buildSecretProviderClassSpec(cfg, staleRefs),
	}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).WithObjects(existing).Build()

	newRefs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"}}
	if err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", newRefs, false); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := &secretsstorev1.SecretProviderClass{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "prod", Name: "muninn-secrets-prod"}, got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	objs := decodeObjects(t, got.Spec)
	if len(objs) != 1 || objs[0].ObjectName != "db_password" {
		t.Errorf("expected updated spec to reflect new refs, got %+v", objs)
	}
}

func TestReconcileSecretProviderClass_Create_GetError_Wrapped(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeCreate)
	boom := errors.New("etcd is on fire")
	c := fake.NewClientBuilder().
		WithScheme(testSPCScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return boom
			},
		}).
		Build()

	err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", nil, false)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped %v, got %v", boom, err)
	}
}

func TestReconcileSecretProviderClass_Reference_NotFound_Rejects(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeReference)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password"}}

	err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false)
	if err == nil {
		t.Fatal("expected an error when the pre-provisioned SPC is missing")
	}
	// The message has to say why nothing was created, not just that a lookup
	// failed: "not found" alone reads like an API error rather than the mode
	// working as configured.
	if !strings.Contains(err.Error(), "no pre-provisioned SecretProviderClass") {
		t.Errorf("expected the error to explain the missing pre-provisioned object, got %v", err)
	}
	if !strings.Contains(err.Error(), "prod") {
		t.Errorf("expected the error to name the namespace, got %v", err)
	}
}

func TestReconcileSecretProviderClass_Reference_Matches_Proceeds(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeReference)
	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"}}
	existing := &secretsstorev1.SecretProviderClass{
		ObjectMeta: objMeta("muninn-secrets-prod", "prod"),
		Spec:       buildSecretProviderClassSpec(cfg, refs),
	}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).WithObjects(existing).Build()

	if err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false); err != nil {
		t.Fatalf("expected match to proceed without error, got %v", err)
	}
}

func TestReconcileSecretProviderClass_Reference_Mismatches_Rejects(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeReference)
	preProvisionedRefs := []SecretRef{{Key: "old_ref", ObjectName: "old", Provider: "vault", Path: "secret/data/prod/old"}}
	existing := &secretsstorev1.SecretProviderClass{
		ObjectMeta: objMeta("muninn-secrets-prod", "prod"),
		Spec:       buildSecretProviderClassSpec(cfg, preProvisionedRefs),
	}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).WithObjects(existing).Build()

	configMapRefs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password"}}
	err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", configMapRefs, false)
	if err == nil {
		t.Fatal("expected an error when the pre-provisioned SPC doesn't match the ConfigMap's refs")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("expected mismatch error, got %v", err)
	}
}

// handWrittenSPC builds a pre-provisioned object the way an operator would,
// from literal YAML rather than by calling the generator under test. A fixture
// produced by buildSecretProviderClassSpec can only ever agree with itself.
func handWrittenSPC(objectsYAML string) *secretsstorev1.SecretProviderClass {
	return &secretsstorev1.SecretProviderClass{
		ObjectMeta: objMeta("muninn-secrets-prod", "prod"),
		Spec: secretsstorev1.SecretProviderClassSpec{
			Provider: "vault",
			Parameters: map[string]string{
				// Deliberately different from the webhook's own config: in
				// Reference mode these belong to whoever wrote this object.
				"vaultAddress": "http://vault.platform-team.svc:8200",
				"roleName":     "platform-provisioned-role",
				"objects":      objectsYAML,
			},
		},
	}
}

func TestReconcileSecretProviderClass_Reference_HandWrittenEquivalent_Proceeds(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeReference)
	refs := []SecretRef{
		{Key: "api_key_ref", ObjectName: "api_key", Provider: "vault", Path: "secret/data/prod/api"},
		{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"},
	}

	// Same secrets, but reversed list order, reversed field order, quoted
	// scalars and four-space indentation. The driver reads all of this
	// identically; only a byte comparison would reject it.
	existing := handWrittenSPC(`
- secretKey: "value"
  secretPath: "secret/data/prod/db-password"
  objectName: "db_password"
- secretPath: "secret/data/prod/api"
  objectName: "api_key"
`)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).WithObjects(existing).Build()

	if err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false); err != nil {
		t.Fatalf("an equivalent hand-written SecretProviderClass must be accepted, got: %v", err)
	}
}

func TestReconcileSecretProviderClass_Reference_ContentMismatches_Rejected(t *testing.T) {
	refs := []SecretRef{
		{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password", SecretKey: "value"},
	}

	tests := []struct {
		name        string
		objectsYAML string
		wantDetail  string
	}{
		{
			name:        "secret is absent",
			objectsYAML: "- objectName: something_else\n  secretPath: secret/data/prod/other\n",
			wantDetail:  "db_password is missing",
		},
		{
			name:        "points at a different path",
			objectsYAML: "- objectName: db_password\n  secretPath: secret/data/staging/db-password\n  secretKey: value\n",
			wantDetail:  "has secretPath",
		},
		{
			name:        "extracts a different field",
			objectsYAML: "- objectName: db_password\n  secretPath: secret/data/prod/db-password\n  secretKey: password\n",
			wantDetail:  "has secretKey",
		},
		{
			// The driver mounts every entry, so an unreferenced one delivers a
			// secret the consumer's config never asked for.
			name: "carries a secret the config never referenced",
			objectsYAML: "- objectName: db_password\n  secretPath: secret/data/prod/db-password\n  secretKey: value\n" +
				"- objectName: stripe_key\n  secretPath: secret/data/prod/stripe\n",
			wantDetail: "stripe_key is not referenced by any config key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testSPCConfig(config.SecretSPCModeReference)
			c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).
				WithObjects(handWrittenSPC(tt.objectsYAML)).Build()

			err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false)
			if err == nil {
				t.Fatal("expected a mismatch to be rejected")
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Errorf("error should name the specific problem %q, got: %v", tt.wantDetail, err)
			}
		})
	}
}

func TestReconcileSecretProviderClass_Reference_WrongProvider_Rejected(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeReference)
	existing := handWrittenSPC("- objectName: db_password\n  secretPath: secret/data/prod/db-password\n")
	existing.Spec.Provider = "azure"
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).WithObjects(existing).Build()

	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password"}}
	err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", refs, false)
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("expected a provider mismatch to be rejected, got %v", err)
	}
}

func TestReconcileSecretProviderClass_Reference_GetError_Wrapped(t *testing.T) {
	cfg := testSPCConfig(config.SecretSPCModeReference)
	boom := errors.New("apiserver unreachable")
	c := fake.NewClientBuilder().
		WithScheme(testSPCScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return boom
			},
		}).
		Build()

	err := ReconcileSecretProviderClass(context.Background(), c, cfg, "prod", nil, false)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped %v, got %v", boom, err)
	}
}

func TestSPCName(t *testing.T) {
	if got := spcName("prod"); got != "muninn-secrets-prod" {
		t.Errorf("spcName(prod) = %q, want muninn-secrets-prod", got)
	}
}

func TestReconcileSecretProviderClass_NotFoundIsAPIStatusError(t *testing.T) {
	// Sanity check on the test setup itself: apierrors.IsNotFound must
	// actually trigger for a genuinely missing object via the fake client,
	// or the Reference-mode "not found" test above would be vacuous.
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()

	got := &secretsstorev1.SecretProviderClass{}
	err := c.Get(context.Background(), client.ObjectKey{Namespace: "prod", Name: "muninn-secrets-prod"}, got)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected IsNotFound, got %v", err)
	}
}

func objMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: namespace}
}
