# Troubleshooting

Issues encountered while building and running Muninn, recorded so the
diagnosis doesn't have to be rediscovered, alongside the failure modes
its integrations produce by design: an admission rejection working as
intended looks much like a malfunction from the outside. This is not an
on-call runbook: Muninn is a portfolio/reference implementation, not an
operated production service.

## A local build silently doesn't run - the deployed Pod is the published image instead

**Symptom:** `make deploy` succeeds, the Pod reaches `1/1 Running`, but
it's running the real published `ghcr.io/garoze/muninn:latest` instead of
a local change - no error anywhere, just code that looks unchanged.

**Cause:** `latest` is the one tag Kubernetes defaults to
`imagePullPolicy: Always` for; every other tag defaults to
`IfNotPresent`. `IMG` defaults to `ghcr.io/garoze/muninn:latest` (the
real published image), so `make image load deploy` run without an `IMG`
override builds and loads a local image under that tag, then `deploy`'s
`Always` default re-pulls the real one from GHCR anyway - the local
build was never used, and nothing about that failure is visible; the Pod
just starts normally.

**Fix:** Always pass a distinct, non-`latest` `IMG` for local testing,
consistently across all three steps:

```bash
make image load IMG=ghcr.io/garoze/muninn:local
make deploy IMG=ghcr.io/garoze/muninn:local
```

A non-`latest` tag's `IfNotPresent` default means the Pod uses what was
just loaded rather than reaching out to the registry at all. The
end-to-end test (`make test-e2e`) already does this consistently via a
package-level constant, used both to build the image and to set the chart's
image values, precisely so this class of mismatch can't happen silently
there either.

## Pod stuck with `ErrImagePull`

**Symptom:** After `make deploy IMG=<tag>`, `kubectl get pods -n
muninn-system` shows the pod stuck in `ErrImagePull`/`ImagePullBackOff`
rather than reaching `1/1 Running`.

**Cause:** The `IMG` value passed to `deploy` doesn't match what was
actually loaded via `make image load` - a typo, or `image`/`load` run
with one `IMG` and `deploy` with another. With a non-`latest` tag's
`IfNotPresent` default, a missing local image means the kubelet falls
back to pulling from the registry, which fails outright for a tag that
was never pushed anywhere (like a local-only `:local` build).

**Fix:** Confirm the exact same `IMG` value was used for `image`, `load`,
and `deploy`, and that `make load`'s import actually succeeded (check
`sudo k3s ctr -n k8s.io images list | grep muninn`).

## Killing a locally-run process doesn't stop it

**Symptom:** Running Muninn via `go run ./cmd/muninn` and killing the
recorded PID doesn't actually stop the server: it keeps holding its
port. The same thing happens with a `kubectl port-forward` started in
the background: it outlives the shell that started it.

**Cause:** `go run` compiles a binary and execs it as a child process;
killing the `go run` process itself doesn't necessarily kill that child.
The same applies to any backgrounded subprocess whose lifecycle isn't
explicitly supervised.

**Fix:** For manual verification, prefer `go build` followed by running
the resulting binary directly, so the PID in hand is the PID actually
listening. For `kubectl port-forward`, confirm it's actually terminated
rather than assuming a single `kill` on the shell job was sufficient.

## Process crashes on first use of a metric, not at startup

**Symptom:** The server starts and runs normally, then panics the first
time one specific operation executes: not during startup, not under
load in general, only the first time that one code path runs.

**Cause:** A Prometheus metric's declared label set and the label values
supplied at its call site have to match in count exactly. A mismatch
compiles fine and doesn't fail at startup: it only panics the first
time that specific metric is actually recorded, which can make the
crash look unrelated to whatever change introduced it.

**Fix:** When adding or changing a metric, verify every call site
supplying label values against that metric's declared label list. This
is exactly the class of bug the pull request template's metrics
checklist item exists to catch before merge, not after.

## `make test-integration` fails before any test runs

**Symptom:** `make test-integration` fails immediately with an error
about missing binaries or an unset environment variable, before any
test logic executes.

**Cause:** The integration tests run against a real, throwaway
`etcd`/`kube-apiserver` pair provisioned by `setup-envtest`, located via
the `KUBEBUILDER_ASSETS` environment variable. If that variable isn't
set, or points at a stale path, the test binaries have no control plane
to start against.

**Fix:** `export KUBEBUILDER_ASSETS=$(setup-envtest use -p path)` (see
the README's Prerequisites section) before running `make
test-integration`.

## Pod stuck in `ContainerCreating` with a `FailedMount` event

**Symptom:** An annotated Pod is admitted, its `SecretProviderClass`
exists, but the Pod never starts. `kubectl describe pod` shows a
`FailedMount` warning naming the `secrets-store.csi.k8s.io` driver.

**Cause:** The failure is on the driver's side of the trust boundary,
not Muninn's. The webhook's only job is producing the
`SecretProviderClass` and the volume that references it; the kubelet and
the driver's provider perform the fetch and the mount. Common causes are
a Vault Kubernetes-auth role not bound to the driver's ServiceAccount, a
`_ref` path that doesn't exist in the store, and the driver or its
provider not being installed on the node at all. The message text comes
from the provider, so it varies by store.

**Fix:** Read the event message first: it distinguishes an authentication
failure from a missing path. Confirm the driver's Node Plugin pods are
running, then check the provider pod's logs on the same node as the
stuck Pod. Verifying the reference by hand against the store (for Vault,
`vault kv get` at the `_ref` path) separates "the secret isn't there"
from "the driver can't authenticate to fetch it".

## Pod creation rejected: `SecretProviderClass ... does not match`

**Symptom:** Creating a Pod fails with `pre-provisioned
SecretProviderClass muninn-secrets-<namespace> does not match ConfigMap
references`, followed by specifics: an object missing, a `secretPath` or
`secretKey` disagreeing, or one `not referenced by any config key`. A
Deployment surfaces this on its ReplicaSet rather than on a Pod, since
no Pod object is created.

**Cause:** This is `SECRET_SPC_MODE=Reference`, working as intended. In
that mode the webhook never writes the object; it compares the
pre-provisioned one against what configuration references and rejects
admission on any disagreement, rather than mounting a stale object.
Extra entries are rejected as well as missing ones, because the driver
mounts everything the object names and an unreferenced entry would
deliver a secret nobody asked for.

**Fix:** Reconcile the two by hand: either correct the pre-provisioned
object, or correct the `_ref`/`_key` entries in the ConfigMap. The
message names each disagreement individually, so it can be worked
through in one pass. Keeping the two in agreement manually is the
accepted cost of this mode; `SECRET_SPC_MODE=Create` derives the object
from configuration instead.

## Pod creation rejected: `secretproviderclasses ... is forbidden`

**Symptom:** In the default `Create` mode, admission is denied with an
RBAC error naming `secretproviderclasses.secrets-store.csi.x-k8s.io`,
typically `cannot create` or `cannot patch`.

**Cause:** The webhook's ServiceAccount is missing the SPC writer role.
Server-Side Apply is authorized via the `patch` verb regardless of
whether the object already exists, so a role granting `update` instead
of `patch` fails on every apply, including the first. Unit tests cannot
catch this: controller-runtime's fake client does not enforce RBAC at
all.

**Fix:** Install with `secrets.enabled=true` and `secrets.spcMode=Create`,
which is what renders that role and its binding. The integration tier covers
both directions of this against an RBAC-enforcing API server, so
`make test-integration` reproduces it without a cluster.

## An annotated Pod has no `/mnt/secrets-store`

**Symptom:** Config lands at `/etc/muninn/config.yaml` as expected, but
the secrets mount is absent entirely and no `SecretProviderClass` was
created.

**Cause:** The CSI volume is injected only when at least one usable
reference is found, so zero usable references means no volume rather
than an empty one. A reference is skipped, with a warning in the
webhook's logs, when its value is not a string, has no `://` scheme, has
a scheme other than `vault`, or is nothing but the `_ref` suffix. A
reference whose ConfigMap is unlabeled or in another namespace is never
resolved in the first place.

**Fix:** Check the webhook's logs for `skipping secret reference`, which
names the offending key and the reason. If nothing was skipped, confirm
the ConfigMap carries the watched label and lives in the Pod's own
namespace: `make query NAMESPACE=<ns> KEYS=<key>_ref` shows whether the
reference resolves at all.

## A mounted secret file contains JSON, not the value

**Symptom:** The file under `/mnt/secrets-store` holds a JSON document
rather than the plain secret, and the application fails parsing it.

**Cause:** No sibling `_key` was given, so the driver mounted the whole
response at that path. KV-shaped stores hold a map at a path rather than
a scalar, and no field name within it is safe to assume, so Muninn
passes no `secretKey` and the driver applies its own documented
whole-secret fallback.

**Fix:** Add the sibling key naming the field to extract, alongside the
reference: `db_password_key: "value"` next to `db_password_ref`. Then
restart the Pod, since the mount cannot change under a running one.

## A newly added `_ref` never appears in a running Pod

**Symptom:** A reference added to a ConfigMap shows up in the resolved
config file, but no corresponding file appears under
`/mnt/secrets-store`. The sidecar logs the new reference.

**Cause:** A CSI mount is fixed for the life of the Pod. The sidecar can
rewrite the config file in place because it owns that file; it cannot
add a volume to a running Pod. This is the documented limitation, not a
failure.

**Fix:** Restart the Pod, which is deliberately left as an operator
decision rather than automated. To see the sidecar's report as a
Kubernetes `Event` rather than only in its logs, the consumer
namespace's ServiceAccount needs `create` on `events.k8s.io`;
`make sample-events` applies a runnable example of that grant.

## `cosign verify` fails with `no matching signatures`

**Symptom:** Verifying a published image or chart reports `no matching
signatures: none of the expected identities matched what was in the
certificate`, and the message goes on to print an identity that looks
identical to the one that was passed.

**Cause:** It is not identical. The signing identity carries the
repository name exactly as GitHub spells it, capitals included, while
every registry reference is lowercase because registries require it.
An identity written by copying the image reference will differ from the
certificate's subject by case alone.

**Fix:** Take the identity from
[`docs/verification.md`](verification.md) rather than deriving it from
the image path. To see what the certificate actually carries, verify with
`--certificate-identity-regexp '.*'` once: the error and the successful
output both print the subject.

## `cosign verify` refuses to run at all

**Symptom:** `--certificate-identity or --certificate-identity-regexp is
required for verification in keyless mode`.

**Cause:** Not a defect. A keyless signature that is merely *valid*
establishes only that somebody signed the artifact. Without an asserted
identity and issuer the check would confirm nothing worth confirming, so
the tool declines rather than producing a reassuring result.

**Fix:** Pass both `--certificate-identity` and
`--certificate-oidc-issuer`.

## `cosign verify-attestation` prints its result and then hangs

**Symptom:** Verification succeeds - the summary and certificate subject
appear - and the command never exits. In a CI job this looks like a hung
step long after the work finished.

**Cause:** The verification summary goes to stderr, and the attestation
payload, around a megabyte, goes to stdout. If nothing drains stdout, the
write blocks and the process waits forever with the verification already
complete.

**Fix:** Redirect stdout to a file and read the payload from there.
Piping into a command that consumes it works too, which is why the
problem hides in interactive use and appears in automation.

## `gh attestation verify` prints nothing

**Symptom:** The command produces no output and exits 0, giving no
indication whether anything was verified.

**Cause:** It prints its human-readable summary only to a terminal.

**Fix:** Treat the exit status as the result, or pass `--format json` and
assert on the predicate type. Automation should do the latter: an exit
status alone does not distinguish a verified provenance statement from
one of a different type.

## An attestation that verified earlier no longer verifies

**Symptom:** An artifact whose SBOM attestation verified shortly after
publication later reports `none of the attestations matched the predicate
type`, naming a different predicate than the one requested.

**Cause:** Attestations attached to an image digest share one location,
and a second writer replaces the first rather than adding to it. When one
of the writers publishes asynchronously, the replacement arrives after
the release has already reported success, so the artifact changes with no
failure anywhere.

**Fix:** Keep one writer per location - this project publishes provenance
to its attestation store rather than to the registry for exactly this
reason. Verify published artifacts on a schedule rather than only during
the release that produced them.

## `helm install` fails on missing `cert-manager.io` resources

**Symptom:** Installing the chart fails with `no matches for kind
"Certificate" in version "cert-manager.io/v1"`, or `no matches for kind
"Issuer"`.

**Cause:** The webhook is enabled by default and needs a certificate the
API server trusts. Its default source is cert-manager, which the chart
expects on the cluster rather than installing.

**Fix:** Install cert-manager, or choose a certificate mode with no
external dependency, or have the chart install cert-manager itself as an
opt-in dependency - see
[`docs/deployment.md`](deployment.md) for the three modes and
[ADR-0015](adr/0015-opt-in-subcharts.md) for why that dependency is off
by default. A fresh cluster enabling it needs the two-phase install
described there: cert-manager's own webhook must be serving before a
`Certificate` is admitted.

## Webhook rejects every Pod after an upgrade

**Symptom:** Installation was working. After a chart upgrade, Pod
creation fails with a TLS error naming an unknown authority, and the
webhook's own Pods may fail to reschedule.

**Cause:** The self-signed certificate mode generates its certificate
authority at install time. An upgrade regenerates it, while the
registered admission configuration still carries the previous bundle, so
the API server no longer trusts the certificate the webhook serves.

**Fix:** For anything beyond a demo cluster, use cert-manager or a
provided certificate instead, both of which survive upgrades. To recover
an installation already in this state, uninstall and reinstall so that
the configuration and the certificate are generated together.

## Pod stuck in `ContainerCreating` after enabling secrets

**Symptom:** With secret delivery enabled, an annotated Pod never starts
and its events report a failed mount referring to a `SecretProviderClass`
or a CSI driver.

**Cause:** Enabling secret delivery grants this project permission to
describe what should be mounted. It does not install the driver that
performs the mount, which is a node-level component and a cluster
singleton.

**Fix:** Install `secrets-store-csi-driver` and a provider for the store
in use, or enable it as an opt-in dependency of the chart. See
[`docs/secret-references.md`](secret-references.md) for the two modes and
what each expects.
