# Changelog

## [0.1.0-alpha.2](https://github.com/Garoze/Muninn/compare/v0.1.0-alpha.1...v0.1.0-alpha.2) (2026-08-15)


### Features

* **build:** add ko configuration ([46b56d9](https://github.com/Garoze/Muninn/commit/46b56d9f4b0131e579ff0ebecbd42f90085c025d))


### Bug Fixes

* **config:** point the manager/webhook Deployments at the published image ([8edb69b](https://github.com/Garoze/Muninn/commit/8edb69b3494133a1fb1eae58b3a33023408aca63))


### Documentation

* update build/deploy docs and troubleshooting for ko ([d56eedc](https://github.com/Garoze/Muninn/commit/d56eedce9ff895abca9f059057d334e5b6f9de4b))

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
