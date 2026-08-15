# Changelog

## [0.1.0-alpha.3](https://github.com/Garoze/Muninn/compare/v0.1.0-alpha.2...v0.1.0-alpha.3) (2026-08-15)


### Features

* **chart:** add helm-unittest, kubeconform, and ct lint to CI ([dce9ad7](https://github.com/Garoze/Muninn/commit/dce9ad70393520b9bd2a4f70d330b6a995823297))
* **chart:** add opt-in cert-manager/CSI-driver dependencies and a webhook enable toggle ([0fbca89](https://github.com/Garoze/Muninn/commit/0fbca89e2f38a9026dbc8122e9dc5cc409a92456))
* **chart:** add the Helm chart's mechanical translation and core values ([cf493a6](https://github.com/Garoze/Muninn/commit/cf493a6928d238854f4b5a7ea3940920934bbc79))
* **ci:** add a nightly workflow installing the published chart and image ([5a51a12](https://github.com/Garoze/Muninn/commit/5a51a120d6ad49a5da141d07c6c99d73fd72dfda))
* **ci:** publish and sign the Helm chart alongside the image ([ec04507](https://github.com/Garoze/Muninn/commit/ec045077ee75ae73af312b4e3c2c313f666194f7))


### Bug Fixes

* **ci:** install helm explicitly before chart-testing-action ([2233071](https://github.com/Garoze/Muninn/commit/2233071b7a97a9753e575cbbbf2eedae82b4f4bc))
* **ci:** register subchart repositories and fetch dependencies ([9deb7ca](https://github.com/Garoze/Muninn/commit/9deb7ca589e4c3e832b4f479a6b943771acdf6d8))
* **ci:** resync develop's release-please manifest after an official cut ([99b5ffa](https://github.com/Garoze/Muninn/commit/99b5ffaff4ca145ff6f99de6e1d675ddbaa306be))


### Documentation

* **testing:** document the nightly tier ([cb1e651](https://github.com/Garoze/Muninn/commit/cb1e6516b6a64b335693d376604a8617313ab269))

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
