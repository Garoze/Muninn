package webhook

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/garoze/muninn/internal/config"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

const (
	spcMismatchFormat = "pre-provisioned SecretProviderClass %s does not match ConfigMap references: %s"
	vaultProvider     = "vault"
)

func spcName(namespace string) string {
	return fmt.Sprintf("muninn-secrets-%s", namespace)
}

type vaultObject struct {
	ObjectName string `yaml:"objectName"`
	SecretPath string `yaml:"secretPath"`
	SecretKey  string `yaml:"secretKey,omitempty"`
}

// vaultObjectsFor derives the driver's per-secret entries from refs. Shared
// with Reference-mode validation so both read the same derivation.
func vaultObjectsFor(refs []SecretRef) []vaultObject {
	objs := make([]vaultObject, 0, len(refs))
	for _, r := range refs {
		objs = append(objs, vaultObject{ObjectName: r.ObjectName, SecretPath: r.Path, SecretKey: r.SecretKey})
	}
	return objs
}

// buildSecretProviderClassSpec derives the object from refs.
func buildSecretProviderClassSpec(cfg *config.Config, refs []SecretRef) secretsstorev1.SecretProviderClassSpec {
	objectsParam, _ := yaml.Marshal(vaultObjectsFor(refs)) // fixed input shape; Marshal on a []vaultObject cannot fail

	return secretsstorev1.SecretProviderClassSpec{
		Provider: secretsstorev1.Provider(vaultProvider),
		Parameters: map[string]string{
			"vaultAddress": cfg.VaultAddress,
			"roleName":     cfg.VaultRoleName,
			"objects":      string(objectsParam),
		},
	}
}

// validateReferencedSpec checks that a pre-provisioned SecretProviderClass
// describes exactly the secrets a namespace's config references.
//
// It compares parsed objects rather than the serialized parameter string. An
// operator writing the object by hand cannot reproduce this package's YAML byte
// for byte, and field order, indentation and quoting carry no meaning to the
// driver. vaultAddress and roleName are not compared at all: in Reference mode
// the object belongs to whoever pre-provisioned it, and those values are theirs
// to set.
//
// An entry the config does not reference is rejected rather than ignored. The
// driver mounts every object in the class into the Pod's volume, so an extra
// entry silently delivers a secret the consumer never asked for.
func validateReferencedSpec(name string, existing secretsstorev1.SecretProviderClassSpec, refs []SecretRef) error {
	if existing.Provider != secretsstorev1.Provider(vaultProvider) {
		return fmt.Errorf(spcMismatchFormat, name,
			fmt.Sprintf("provider is %q, want %q", existing.Provider, vaultProvider))
	}

	var have []vaultObject
	if err := yaml.Unmarshal([]byte(existing.Parameters["objects"]), &have); err != nil {
		return fmt.Errorf(spcMismatchFormat, name,
			fmt.Sprintf("its objects parameter is not a list of secret objects: %v", err))
	}

	haveByName := make(map[string]vaultObject, len(have))
	for _, o := range have {
		haveByName[o.ObjectName] = o
	}

	// refs arrive sorted by key, so these read out deterministically.
	var problems []string
	for _, want := range vaultObjectsFor(refs) {
		got, ok := haveByName[want.ObjectName]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is missing", want.ObjectName))
			continue
		}
		delete(haveByName, want.ObjectName)

		if got.SecretPath != want.SecretPath {
			problems = append(problems, fmt.Sprintf("%s has secretPath %q, want %q", want.ObjectName, got.SecretPath, want.SecretPath))
		}
		if got.SecretKey != want.SecretKey {
			problems = append(problems, fmt.Sprintf("%s has secretKey %q, want %q", want.ObjectName, got.SecretKey, want.SecretKey))
		}
	}

	extra := make([]string, 0, len(haveByName))
	for objectName := range haveByName {
		extra = append(extra, objectName)
	}
	sort.Strings(extra)
	for _, objectName := range extra {
		problems = append(problems, fmt.Sprintf("%s is not referenced by any config key", objectName))
	}

	if len(problems) > 0 {
		return fmt.Errorf(spcMismatchFormat, name, strings.Join(problems, "; "))
	}
	return nil
}

// ReconcileSecretProviderClass ensures (Create mode) or validates
// (Reference mode) the namespace's SecretProviderClass against refs.
//
// dryRun suppresses the only branch that writes. Reference mode reads and
// compares, so it still runs under a dry run and still rejects a mismatch.
func ReconcileSecretProviderClass(ctx context.Context, c client.Client, cfg *config.Config, namespace string, refs []SecretRef, dryRun bool) error {
	name := spcName(namespace)
	wantSpec := buildSecretProviderClassSpec(cfg, refs)

	existing := &secretsstorev1.SecretProviderClass{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, existing)

	if cfg.SecretSPCMode == config.SecretSPCModeReference {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf(
				"no pre-provisioned SecretProviderClass %s in namespace %s, and %s mode does not create one",
				name, namespace, config.SecretSPCModeReference)
		}

		if err != nil {
			return fmt.Errorf("getting SecretProviderClass %s: %w", name, err)
		}

		return validateReferencedSpec(name, existing.Spec, refs)
	}

	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("getting SecretProviderClass %s: %w", name, err)
	}

	if err == nil && equality.Semantic.DeepEqual(existing.Spec, wantSpec) {
		return nil
	}

	// Create is the only branch that writes, so it is the only one a dry-run
	// admission has to stop short of.
	if dryRun {
		return nil
	}

	// Always a fresh object, never the fetched "existing" one: reusing it
	// would carry its ResourceVersion into the apply-patch body, which
	// turns concurrent admissions racing to write this object into 409
	// conflicts instead of the conflict-free merge Server-Side Apply is
	// meant to provide here.
	spc := &secretsstorev1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       wantSpec,
	}
	return applySecretProviderClass(ctx, c, spc)
}

func applySecretProviderClass(ctx context.Context, c client.Client, spc *secretsstorev1.SecretProviderClass) error {
	spc.APIVersion = secretsstorev1.GroupVersion.String()
	spc.Kind = "SecretProviderClass"

	// client.Apply is deprecated in favor of the typed Client.Apply(), which
	// needs a generated runtime.ApplyConfiguration type - secrets-store-csi-driver
	// ships none, so this patch-type form is the only SSA path available for it.
	//nolint:staticcheck // SA1019: see above, no typed ApplyConfiguration exists for this type
	return c.Patch(ctx, spc, client.Apply, client.ForceOwnership, client.FieldOwner("muninn-webhook"))
}
