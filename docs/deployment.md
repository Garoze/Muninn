# Deployment

Muninn runs in two modes, from the same image: `serve`, the resolver behind the
gRPC API, and `webhook`, the mutating admission webhook that delivers resolved
configuration into Pods. Both come from one chart release, and neither requires
the other to be running.

The chart is the deployment surface. `make deploy` wraps it for local
development against a locally built image; a cluster consuming a published
release installs the chart directly, as the [README](../README.md) shows.
`helm show values oci://ghcr.io/garoze/charts/muninn` documents every value,
each one commented with the reasoning behind its default.

`make run` runs the resolver as a host process against `$KUBECONFIG`, which is
suited to development. Running the binary directly requires
`KUBE_CONFIG_PATH`, which is separate from `kubectl`'s `$KUBECONFIG`:

```bash
KUBE_CONFIG_PATH=~/.kube/config go run ./cmd/muninn serve
```

## The resolver

`make deploy` runs Muninn as a Pod under its own least-privilege
`ServiceAccount`, granted `configmaps` and nothing else. The default `IMG`
is the published `ghcr.io/garoze/muninn:latest`, so on a cluster that can
reach GHCR this is the entire deployment:

```bash
make deploy     # install or upgrade the chart, pulling the published image
```

The webhook is enabled by default and needs cert-manager, so on a cluster
without it, install the resolver alone:

```bash
make deploy HELM_ARGS="--set webhook.enabled=false"
```

Re-running `make deploy` upgrades the existing release rather than failing,
so changing a value is the same command with a different `HELM_ARGS`.

To run a local build instead - developing against a change not published
yet - build and load it under a distinct tag first, then point `deploy` at
that tag. A tag other than `latest` defaults to `imagePullPolicy:
IfNotPresent`, so the Pod uses what was just loaded rather than pulling the
real image out from under it:

```bash
make image IMG=ghcr.io/garoze/muninn:local   # build via ko into bin/image.tar
make load                                     # import it into k3s's containerd store
make deploy IMG=ghcr.io/garoze/muninn:local   # install, pointed at the local build
```

> [!NOTE]
> `make load` imports into k3s specifically. On another cluster, get
> `bin/image.tar` onto the nodes however that cluster expects (`minikube
> image load`, `kind load image-archive`, or push the built image to a
> registry the cluster can reach), then run `make deploy IMG=<that
> reference>`.

```bash
kubectl get pods -n muninn-system   # should reach 1/1 Running
kubectl port-forward -n muninn-system deploy/muninn 5010:5010 &
make query NAMESPACE=arasaka KEYS=LOG_LEVEL
```

The Pod reports `NOT_SERVING` on its health port until every registered
source's informer has completed its initial list and watch, so it does not
receive traffic with an incomplete cache. `make undeploy` tears it back down.

## The admission webhook

The webhook is a separate Deployment with its own `ServiceAccount`, installed
by the same release and enabled by default. Its serving certificate comes from
[cert-manager](https://cert-manager.io/), which the chart expects on the
cluster rather than installing:

```bash
make deploy     # webhook.enabled defaults to true
```

Two alternatives to that default exist for clusters without cert-manager: the
chart can generate a self-signed CA itself, or take an existing certificate
and CA bundle. `certificate.mode` in `values.yaml` covers all three, including
the rotation hazard the self-signed mode carries across upgrades.

A Pod opts in through an annotation; nothing else in its spec changes:

```yaml
metadata:
  annotations:
    muninn.io/inject: "true"
```

At admission the webhook injects a shared volume, an init container that
resolves the namespace once, and a sidecar that refreshes the file on an
interval. It also mounts that volume into the Pod's existing containers, so the
application reads `/etc/muninn/config.yaml` with no gRPC client of its own.

The webhook resolves from its own watcher and cache rather than calling the
resolver over gRPC. A `MutatingWebhookConfiguration` with `failurePolicy: Fail`
blocks Pod scheduling when the webhook is unreachable, so introducing a network
dependency on another service at admission time would widen that blast radius;
see [ADR-0010](adr/0010-single-process-webhook.md) and `design.md`'s
availability boundaries.

> [!IMPORTANT]
> Because of that `failurePolicy`, an unreachable webhook blocks Pod creation
> across every namespace it matches, not only annotated Pods. The chart always
> excludes `kube-system` and the release's own namespace, without which an
> unavailable webhook would block its own replacement Pod. On a cluster
> running anything else, narrow it further with
> `webhook.excludedNamespaces`, or set `webhook.failurePolicy=Ignore` to make
> injection best-effort instead.

`MUNINN_INJECT_IMAGE` must match the webhook's own Deployment image, since a
container cannot read its own image through the Downward API. Both come from
the same chart value, so they cannot disagree.

## Secret references

Delivering the values a configuration references additionally requires
[`secrets-store-csi-driver`](https://secrets-store-csi-driver.sigs.k8s.io/) and
a supported provider on the cluster. Both are external prerequisites this
repository does not install, in the same category as cert-manager above.

It is also off by default in the chart, because the `Create` mode needs a
grant to write `SecretProviderClass` objects into arbitrary consumer
namespaces - which is precisely what `Reference` mode exists to avoid, so it
is not granted unasked:

```bash
make deploy HELM_ARGS="--set secrets.enabled=true"
```

See [`secret-references.md`](secret-references.md) for the two modes and what
each one expects.
