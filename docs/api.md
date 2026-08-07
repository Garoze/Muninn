# API

`discovery.v1.DiscoveryService` exposes three RPCs, all namespace-scoped. The
schema lives in [`proto/v1/discovery.proto`](../proto/v1/discovery.proto).

| RPC | Purpose |
|---|---|
| `Query` | Resolve named keys for a namespace. Absent keys come back in `missing_keys`; `strict` turns them into `InvalidArgument` instead. |
| `Resolve` | Return everything currently resolved for a namespace, with no keys enumerated up front. Backs the injected init container and sidecar, which cannot know the key set ahead of time. |
| `Describe` | List the active configuration sources: kind, label selector and scope. |

`Query` and `Resolve` are separate rather than one RPC with an empty key list
standing in for "everything", which reads ambiguously as either a wildcard or a
caller bug. [ADR-0011](adr/0011-resolve-rpc.md) records that decision, and
[ADR-0009](adr/0009-no-fixed-key-whitelist.md) why no fixed key vocabulary is
validated against.

Each returned value carries the source object it was resolved from, and each
response carries a cache revision token useful when reconciling what a caller
observed against what the cache held.

## muninnctl

`muninnctl` is the first-party client, in the kubectl idiom:

```bash
go run ./cmd/muninnctl describe
go run ./cmd/muninnctl query --namespace arasaka --keys LOG_LEVEL,FEATURE_DARKMODE
```

`make describe` and `make query NAMESPACE=<ns> KEYS=<a,b,c>` wrap the same two
commands. Both accept `--addr` and, against a TLS-enabled server, `--tls-ca`.

## grpcurl

The server registers gRPC reflection, so [`grpcurl`](https://github.com/fullstorydev/grpcurl)
needs no `.proto` files:

```bash
grpcurl -plaintext localhost:5010 list discovery.v1.DiscoveryService

grpcurl -plaintext localhost:5010 discovery.v1.DiscoveryService/Describe

grpcurl -plaintext -d '{
  "namespace": "arasaka",
  "keys": ["LOG_LEVEL", "FEATURE_DARKMODE"]
}' localhost:5010 discovery.v1.DiscoveryService/Query

grpcurl -plaintext -d '{"namespace": "arasaka"}' \
  localhost:5010 discovery.v1.DiscoveryService/Resolve
```

Drop `-plaintext` and pass `-cacert` when the server runs with
`GRPC_TLS_CERT_PATH`/`GRPC_TLS_KEY_PATH` set.

## Health

A gRPC health service runs on a separate port (`GRPC_PROBE_ADDR`, default
`:5011`), reporting `NOT_SERVING` until every registered source's informer has
synced:

```bash
grpcurl -plaintext localhost:5011 grpc.health.v1.Health/Check
```

## No client library

Muninn ships no importable Go SDK. Layered configuration merging on the
consuming side is a separable problem from watching, caching and serving
configuration, and the admission webhook covers the case for consumers who do
not want to write gRPC client code at all. See
[ADR-0006](adr/0006-no-client-library.md).
