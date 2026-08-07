# Deployment

Muninn runs in two modes, from the same image: `serve`, the resolver behind the
gRPC API, and `webhook`, the mutating admission webhook that delivers resolved
configuration into Pods. Each is deployed separately, and neither requires the
other to be running.

`make run` runs the resolver as a host process against `$KUBECONFIG`, which is
suited to development. Running the binary directly requires
`KUBE_CONFIG_PATH`, which is separate from `kubectl`'s `$KUBECONFIG`:

```bash
KUBE_CONFIG_PATH=~/.kube/config go run ./cmd/muninn serve
```

## The resolver

`make deploy` runs Muninn as a Pod under its own least-privilege
`ServiceAccount`, granted `configmaps` and nothing else:

```bash
make image      # build the image
make load       # import it into k3s's containerd store
make deploy     # apply config/manager/ + config/rbac/
```

> [!NOTE]
> `make load` imports into k3s specifically. On another cluster, get
> `localhost/muninn:latest` onto the nodes however that cluster expects
> (`minikube image load`, `kind load image-archive` after a `podman save`, or
> a push to a registry the cluster can reach), then run `make deploy`.

```bash
kubectl get pods -n muninn-system   # should reach 1/1 Running
kubectl port-forward -n muninn-system deploy/muninn 5010:5010 &
make query NAMESPACE=arasaka KEYS=LOG_LEVEL
```

The Pod reports `NOT_SERVING` on its health port until every registered
source's informer has completed its initial list and watch, so it does not
receive traffic with an incomplete cache. `make undeploy` tears it back down.

## The admission webhook

`make deploy` covers the gRPC API only. The webhook is a separate Deployment,
with its own `ServiceAccount`, and requires
[cert-manager](https://cert-manager.io/) on the cluster: Muninn issues its
serving `Certificate` through cert-manager but does not install it.

```bash
make deploy-webhook     # apply config/webhook/: Issuer, Certificate,
                        # ServiceAccount, RBAC, Service, Deployment,
                        # MutatingWebhookConfiguration
```

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
availability boundaries. `make undeploy-webhook` tears it back down.

> [!IMPORTANT]
> Because of that `failurePolicy`, scope the `MutatingWebhookConfiguration`
> with a `namespaceSelector` when first deploying it to a cluster running
> anything else. An unreachable webhook otherwise blocks Pod creation across
> every namespace it matches, not only annotated Pods.

`MUNINN_INJECT_IMAGE` must match the webhook's own Deployment image, since a
container cannot read its own image through the Downward API. The manifest
keeps the two agreeing with a YAML anchor rather than by hand.

## Secret references

Delivering the values a configuration references additionally requires
[`secrets-store-csi-driver`](https://secrets-store-csi-driver.sigs.k8s.io/) and
a supported provider on the cluster. Both are external prerequisites this
repository does not install, in the same category as cert-manager above. See
[`secret-references.md`](secret-references.md).
