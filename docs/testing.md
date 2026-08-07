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
- Clients bound to the real manifests' RBAC, in both `SECRET_SPC_MODE`
  postures. `envtest` enforces RBAC; the fake client does not, and a missing
  verb here fails a test instead of a cluster.

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
export KUBEBUILDER_ASSETS=$(setup-envtest use -p path)
make test-integration
```

## End-to-end

```bash
make image load         # `make load` requires interactive sudo
make test-e2e           # against an existing cluster

make test-e2e-csi       # provisions a disposable kind cluster, then removes it
```

`make test-e2e` deploys through the same `make deploy` and `make deploy-webhook`
targets an operator uses, then verifies the gRPC API, injection into an
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

## Verified manually

Two narrower claims are checked by hand against a real cluster: that an
*unannotated* Pod schedules untouched, and that `failurePolicy: Fail` causes no
collateral blast radius across unrelated Pods. Automating them the way the
tiers above are automated is possible and has no design obstacle; it is simply
not built. Until it is, they carry no CI signal protecting them from regressing
silently, which `design.md` records as an accepted tradeoff rather than an
oversight.
