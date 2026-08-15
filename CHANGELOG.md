# Changelog

## [0.1.0-alpha.3](https://github.com/Garoze/Muninn/compare/v0.1.0-alpha.2...v0.1.0-alpha.3) (2026-08-15)


### Features

* **chart:** add helm-unittest, kubeconform, and ct lint to CI ([dce9ad7](https://github.com/Garoze/Muninn/commit/dce9ad70393520b9bd2a4f70d330b6a995823297))
* **chart:** add opt-in cert-manager/CSI-driver dependencies and a webhook enable toggle ([0fbca89](https://github.com/Garoze/Muninn/commit/0fbca89e2f38a9026dbc8122e9dc5cc409a92456))
* **chart:** add the Helm chart's mechanical translation and core values ([cf493a6](https://github.com/Garoze/Muninn/commit/cf493a6928d238854f4b5a7ea3940920934bbc79))
* **ci:** add a nightly workflow installing the published chart and image ([b3b80bd](https://github.com/Garoze/Muninn/commit/b3b80bd227985cfaaa4f4e8d20ccf70e3f9a7c14))
* **ci:** publish and sign the Helm chart alongside the image ([4bb9d3b](https://github.com/Garoze/Muninn/commit/4bb9d3bd3add95018c0963c93e5129b784cf3f6a))


### Bug Fixes

* **ci:** install helm explicitly before chart-testing-action ([5100e16](https://github.com/Garoze/Muninn/commit/5100e16a9966b5b84141c6b2a3b02033187a80ce))
* **ci:** register subchart repositories and fetch dependencies ([dcf6476](https://github.com/Garoze/Muninn/commit/dcf64766ec7c128a3b6c90cd0e1204139b7dce6e))
* **ci:** resync develop's release-please manifest after an official cut ([b1106e6](https://github.com/Garoze/Muninn/commit/b1106e6a2cb26e2fed90aaccb674f53dcb12b9aa))


### Documentation

* **testing:** document the nightly tier ([d9b7274](https://github.com/Garoze/Muninn/commit/d9b727433ba2a21d85d0fe4da11a94bc2afb20dc))

## [0.1.0-alpha.2](https://github.com/Garoze/Muninn/compare/v0.1.0-alpha.1...v0.1.0-alpha.2) (2026-08-15)


### Features

* **build:** add ko configuration ([46b56d9](https://github.com/Garoze/Muninn/commit/46b56d9f4b0131e579ff0ebecbd42f90085c025d))


### Bug Fixes

* **config:** point the manager/webhook Deployments at the published image ([8edb69b](https://github.com/Garoze/Muninn/commit/8edb69b3494133a1fb1eae58b3a33023408aca63))


### Documentation

* update build/deploy docs and troubleshooting for ko ([d56eedc](https://github.com/Garoze/Muninn/commit/d56eedce9ff895abca9f059057d334e5b6f9de4b))

## 0.2.0 (2026-08-15)

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
