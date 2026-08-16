# Testing

Four tiers, separated by what each one needs to run and what only it can prove.
`design.md`'s testing strategy section records the reasoning; this document is
how to run them.

```bash
make test-unit          # no cluster required
make test-integration   # envtest: a throwaway etcd + kube-apiserver
make test               # both, and what CI runs
```

## Unit

Covers the domain layer, the gRPC translation boundary, the watch-and-patch
logic, the webhook's patch construction and `SecretProviderClass` derivation,
observability wiring and configuration parsing. No cluster, no network.

The webhook's Kubernetes interactions are unit-tested against
controller-runtime's fake client. That client does not enforce RBAC, which is
exactly why the tiers below exist.

## Integration

`make test-integration` runs against a real `etcd` and `kube-apiserver` started
by [`envtest`](https://pkg.go.dev/sigs.k8s.io/controller-runtime/tools/setup-envtest),
covering behavior a fake client cannot show:

- Projection of a real ConfigMap into the cache, updates, per-source deletion,
  and label-selector scoping.
- Two sources of the same kind co-registered, which is what the cache's
  per-source keying exists to keep separate.
- The webhook resolving from its own cache with the resolver absent entirely,
  which is the actual proof of the availability decoupling rather than a
  restatement of it.
- Clients bound to the RBAC the chart renders, in both `SECRET_SPC_MODE`
  postures. `envtest` enforces RBAC; the fake client does not, and a missing
  verb here fails a test instead of a cluster. The chart's own unit tests
  cover which grants each mode renders, not whether those grants authorize
  the write they exist for; that question needs an API server.

Rendering the chart puts `helm` on this tier's prerequisites; without it those
tests skip.

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.24.1
export KUBEBUILDER_ASSETS=$(setup-envtest use -p path)
make test-integration
```

## End-to-end

```bash
make test-e2e           # against an existing cluster; builds and pushes for you
make test-e2e-csi       # provisions a disposable kind cluster, then removes it
```

`make test-e2e` builds the image into a local registry and points the chart
at it, which needs no root. Importing into k3s's containerd does, because its
socket is owned by root, and that is the only thing `make load` needs a
password for. The registry keeps running between runs; `make registry-stop`
removes it.

This works for a node that shares this host's network namespace, which
resolves `localhost:5000` to that registry - k3s, `minikube --driver=none`,
or kind with the port mapped. For a cluster anywhere else, build a tarball
and load it however that cluster expects, then point the test at the result:

```bash
make image load IMG=ghcr.io/garoze/muninn:local    # k3s; needs sudo
MUNINN_E2E_IMAGE=ghcr.io/garoze/muninn:local make test-e2e
```

`make test-e2e` installs the chart an operator installs, enabling the webhook
through the same follow-up upgrade a cluster without cert-manager already
serving has to perform, then verifies the gRPC API, injection into an
annotated Pod, and that a ConfigMap edit reaches the mounted file without a
restart.

`make test-e2e-csi` adds the CSI path on a cluster it provisions itself
(`kind`, `secrets-store-csi-driver`, Vault in dev mode): a webhook-generated
`SecretProviderClass`, a Vault secret and the config file landing in one Pod
together, and the sidecar reporting a newly-added reference. It needs `kind`,
`helm`, `kubectl` and a container engine on `PATH`, and takes several minutes.

Neither end-to-end tier runs in CI. Both require a real cluster, and the CSI
tier provisions its own; running them per commit would cost minutes for signal
that changes only when the deployment path does. They are gated behind
environment variables (`MUNINN_IT_E2E`, `MUNINN_IT_CSI_E2E`) so an ordinary
`go test ./...` skips them.

## Nightly

A scheduled workflow installs the chart and image from GHCR, not this
checkout, onto a disposable k3s cluster (`k3d`). This is the only tier that
tests the published artifact rather than the source: a chart that renders
correctly but references an image tag that was never pushed, a GHCR package
flipped back to private, or a chart version whose OCI push didn't match
`Chart.yaml` are all invisible to every other tier. Both signatures and both
of the image's attestations are verified before anything is installed.

Attestations are checked here rather than only at publication because the
release job cannot prove one survived. An attestation attached to an image
digest can be replaced after the job that produced it has already reported
success, which is a failure no green release can observe. This tier reads
the published artifact long after the fact, which is the only position from
which that is visible. The two are read with different tools: the SBOM is
attached to the image digest, provenance lives in this repository's
attestation store.

Two cells: cert-manager already present externally (the common case), and a
bare cluster exercising the two-phase install the opt-in `cert-manager`/
`secrets-store-csi-driver` subcharts require. Both install into a
non-default namespace and delete the webhook Pod, confirming the
replacement schedules - the case where a hardcoded namespace exclusion in
the `MutatingWebhookConfiguration` would silently fail under
`failurePolicy: Fail`.

Not yet covered: the wider Kubernetes-version/cert-mode/secrets matrix, and
upgrading a previously published chart version into the current one. Several
chart versions are published now, so the upgrade case is testable - and it is
the only route to the self-signed certificate mode's rotation hazard, which
no tier reaches today.

## Verified manually

Two narrower claims are checked by hand against a real cluster: that an
*unannotated* Pod schedules untouched, and that `failurePolicy: Fail` causes no
collateral blast radius across unrelated Pods. Automating them the way the
tiers above are automated is possible and has no design obstacle; it is simply
not built. Until it is, they carry no CI signal protecting them from regressing
silently, which `design.md` records as an accepted tradeoff rather than an
oversight.
