# Changelog

## 0.1.0-alpha.1 (2026-08-15)

Initial release. Muninn watches labeled ConfigMaps behind a pluggable
`ConfigSource` interface, merges them into a namespace-scoped in-memory
cache, and exposes the result over a gRPC `Query`/`Resolve`/`Describe` API.

### Features

* ConfigMap aggregation via a pluggable `ConfigSource` interface, with a
  patch-based merge so one source's update never clobbers another's
* gRPC discovery API - `Query` for named keys, `Resolve` for a whole
  namespace, `Describe` for the active sources' shape
* A mutating admission webhook that delivers resolved configuration into a
  Pod as a file, kept current by a sidecar, with no client code required
* Secret references resolved via `secrets-store-csi-driver` - configuration
  carries a reference, never a value; the gRPC API stays unauthenticated by
  design and never sees secret data
* Prometheus metrics and OpenTelemetry tracing on every gRPC call and
  admission request
* `muninn` (serve/webhook/resolve) and `muninnctl` CLIs, both versioned via
  build-time `-ldflags` stamping
* Signed, published container images on GHCR (keyless cosign signing,
  verified in CI before the workflow completes)

See [`docs/design.md`](docs/design.md) and [`docs/adr/`](docs/adr/) for the
architecture and the decisions behind it.
