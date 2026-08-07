# Secret references

Muninn never carries a secret value. Configuration holds a *reference* to one,
and [`secrets-store-csi-driver`](https://secrets-store-csi-driver.sigs.k8s.io/)
fetches the value and mounts it into the Pod directly. Muninn's only
responsibility is translating references into the object that driver reads.

[ADR-0012](adr/0012-csi-secret-delivery.md) covers why the boundary is drawn
there, with a trust-boundary diagram, and `design.md`'s secret delivery section
records the alternatives rejected.

## Prerequisites

Beyond what the admission webhook already requires (see
[`deployment.md`](deployment.md)):

| Dependency | Role |
|---|---|
| [`secrets-store-csi-driver`](https://secrets-store-csi-driver.sigs.k8s.io/) | The Node Plugin DaemonSet that performs the mount |
| A supported provider | Resolves a reference against a real store. [Vault](https://www.vaultproject.io/) is the provider implemented here |

Neither is installed by this repository, in the same category as cert-manager.

## The reference convention

A reference is a configuration key ending in `_ref`. Two optional keys sharing
the same prefix qualify it:

```yaml
data:
  db_password_ref:  "vault://secret/data/arasaka/db-password"  # required
  db_password_key:  "value"                                    # optional
  db_password_file: "/mnt/secrets-store/db_password"           # optional
```

- **`_ref`** names where the secret lives. Its URI scheme selects the provider;
  `vault` is the only scheme implemented. An unrecognized scheme is skipped and
  logged rather than failing admission, matching how the webhook treats every
  other data problem it understands.
- **`_key`** picks one field out of the secret at that path. KV-shaped stores
  hold a map at a path, not a scalar, and no field name is safe to assume;
  without `_key` the mounted file holds the whole response as JSON, which is
  the driver's own documented behavior rather than something Muninn
  reimplements.
- **`_file`** is documentation only. Muninn does not read it. The mount path is
  always `/mnt/secrets-store/` followed by the `_ref` key with `_ref` stripped,
  so the example above resolves to `/mnt/secrets-store/db_password`.

## What the webhook does with them

At admission the webhook derives a `SecretProviderClass` named
`muninn-secrets-<namespace>` from every reference resolved for that namespace,
and injects a read-only CSI volume bound to it alongside the existing config
volume. One object per namespace, not per Pod: every Pod in a namespace
resolves the same references, so a per-Pod object would be identical copies
needing collection.

The object the driver creates *per* Pod to track mount state
(`SecretProviderClassPodStatus`) belongs to the driver, is owner-referenced to
the Pod, and is never touched by Muninn.

A Pod opts in through the same `muninn.io/inject: "true"` annotation, and its
`ServiceAccount` must be bound to a role in the secret store. That binding is
configured once per namespace, not per secret. The application then reads
files:

```bash
cat /etc/muninn/config.yaml         # config, as before
cat /mnt/secrets-store/db_password  # the secret value
```

## Who owns the SecretProviderClass

`SECRET_SPC_MODE` selects between two postures:

| Mode | Behavior |
|---|---|
| `Create` (default) | The webhook creates the object, and updates it through Server-Side Apply when a namespace's references change. Configuration stays the single place an operator declares what a namespace needs. |
| `Reference` | A platform team pre-provisions the object; the webhook only reads it, and rejects admission with the mismatch when it does not describe what configuration references. Requires no write access, at the cost of two objects an operator keeps in agreement. |

`Reference` validation compares derived content, not serialized form, since a
hand-written object cannot reproduce a generated serialization and formatting
means nothing to the driver. An entry configuration does not reference is still
rejected: the driver mounts every entry the object names, so an unreferenced
one would deliver a secret nobody asked for.

## A mount is fixed for the Pod's lifetime

A CSI mount cannot change after the Pod is running, so a reference added to a
running Pod's ConfigMap cannot be applied retroactively. The sidecar detects
it, logs it, and emits a Kubernetes `Event` where RBAC allows; acting on it
means restarting the Pod, which is an operator's decision rather than something
to do silently.

Failure modes on this path, from a rejected admission to a Pod stuck on a
`FailedMount`, are collected in
[`troubleshooting.md`](troubleshooting.md).

That `Event` is written under the *consumer* Pod's own `ServiceAccount`, not
the webhook's, so it requires an explicit grant in the consumer's namespace.
`make sample-events` applies a runnable example of exactly that grant, scoped
to the sample namespace. Without it the sidecar still logs; the `Event` attempt
fails closed to a log line rather than a crash or a lost signal.
