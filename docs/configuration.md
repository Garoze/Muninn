# Configuration

Every setting is an environment variable with a default, so both modes run
unconfigured against a local cluster. `serve` and `webhook` read the same set;
each ignores what it does not use.

## Resolver (`muninn serve`)

| Variable | Default | Purpose |
|---|---|---|
| `KUBE_CONFIG_PATH` | *(unset)* | Path to a kubeconfig. Unset uses in-cluster credentials. |
| `CONFIGMAP_LABEL_SELECTOR` | `muninn.io/config=runtime` | Scopes which ConfigMaps are watched. |
| `ENABLED_CONFIG_SOURCES` | *(unset)* | Comma-separated `Kind` names. Unset enables every registered source; can only narrow, never add. |
| `GRPC_SERVICE_ADDR` | `:5010` | gRPC API bind address. |
| `GRPC_PROBE_ADDR` | `:5011` | gRPC health probe bind address. |
| `GRPC_TLS_CERT_PATH` | *(unset)* | Enables TLS on the gRPC API. Must be set together with the key. |
| `GRPC_TLS_KEY_PATH` | *(unset)* | Private key paired with the above. Setting one alone is an error. |
| `CACHE_ENTRY_TTL` | `0` | Rejects entries older than this. `0` disables staleness checks. |
| `STARTUP_TIMEOUT` | `2m` | Budget for startup, including the initial informer sync. |

## Webhook (`muninn webhook`)

| Variable | Default | Purpose |
|---|---|---|
| `MUNINN_INJECT_IMAGE` | *(required)* | Image stamped onto injected containers. Must match the webhook's own Deployment image; startup fails if unset. |
| `MUNINN_SELF_ADDR` | `muninn.muninn-system.svc.cluster.local:5010` | Address the injected containers dial. |
| `WEBHOOK_ADDR` | `:8443` | HTTPS bind address. |
| `WEBHOOK_TLS_CERT_PATH` | `/etc/webhook/certs/tls.crt` | Serving certificate, written by cert-manager. |
| `WEBHOOK_TLS_KEY_PATH` | `/etc/webhook/certs/tls.key` | Private key paired with the above. |
| `SECRET_SPC_MODE` | `Create` | `Create` generates the `SecretProviderClass`; `Reference` expects a pre-provisioned one and only validates it. Case-insensitive; an unrecognized value fails startup. |
| `VAULT_ADDRESS` | `http://vault.kube-system:8200` | Vault address written into the generated `SecretProviderClass`. |
| `VAULT_ROLE_NAME` | `muninn` | Vault Kubernetes-auth role bound to the CSI driver's ServiceAccount. |

## Both modes

| Variable | Default | Purpose |
|---|---|---|
| `METRICS_ADDR` | `:9090` | Prometheus `/metrics` bind address. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTLP trace endpoint (host:port, no scheme). |
| `OTEL_TRACES_SAMPLE_ARG` | `0.1` | Root-span sample ratio. A caller's own sampling decision is honored when present. |

## TLS

The two servers are not symmetric. TLS on the gRPC API is opt-in and off by
default, on the assumption that a service mesh may already terminate it. The
webhook's TLS is not optional, because the Kubernetes API server calls
admission webhooks over TLS unconditionally. See
[`design.md`](design.md) for why.

## Restricting which sources run

`ENABLED_CONFIG_SOURCES` narrows the registered `ConfigSource`s to a named
subset, by `Kind()`:

```bash
# only run the ConfigMap source, today's default, made explicit
ENABLED_CONFIG_SOURCES=ConfigMap make run
```

`Describe` reflects the filter: it lists only sources that are both registered
in code and named here. Naming a kind that is not registered leaves nothing
enabled and fails startup, rather than running with no sources watched.
