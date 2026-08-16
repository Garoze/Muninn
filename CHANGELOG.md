# Changelog

## [0.2.4-alpha](https://github.com/Garoze/Muninn/compare/v0.2.3...v0.2.4-alpha) (2026-08-16)


### Bug Fixes

* **ci:** close two defects the audit found in the release path ([326097f](https://github.com/Garoze/Muninn/commit/326097fc384c06ef7a025ef985c40fd0213e4f15))
* close the audit's chart, permission and documentation findings ([d09b50b](https://github.com/Garoze/Muninn/commit/d09b50b12a9f3f78d417b6ce137c3111b4c001c4))
* close the audit's remaining behavioural findings ([6a33b31](https://github.com/Garoze/Muninn/commit/6a33b31fa046aa358ab4877ed88cefef6249e62f))
* close the audit's remaining minor findings ([49abdbf](https://github.com/Garoze/Muninn/commit/49abdbfff54ec05f8557dba5466a159a7c5d6835))
* correct two claims the audit found and bump the chart ([402c4de](https://github.com/Garoze/Muninn/commit/402c4deb526036bfd1cb9b4263edd55af58110dd))

## [0.2.3](https://github.com/Garoze/Muninn/compare/v0.2.2...v0.2.3) (2026-08-16)


### Features

* **chart:** allow the image to be pinned by digest ([ae6ef1c](https://github.com/Garoze/Muninn/commit/ae6ef1c4125869c64c3a093fbd610dde533b5a89))


### Bug Fixes

* **ci:** create a merge commit when main is strictly ahead ([25184bd](https://github.com/Garoze/Muninn/commit/25184bd0a513b540b7d51a1145c7f4bbc2ff059d))
* **ci:** correct main's release state for v0.2.2 ([4a3344d](https://github.com/Garoze/Muninn/commit/4a3344d31a55a3dad1e4df2bee4fca76e7fb2fe5))
* **ci:** consolidate prerelease entries with no increment ([d9b51e3](https://github.com/Garoze/Muninn/commit/d9b51e32451f5d18934a2281cf042d822e1e55ef))

## 0.2.2 (2026-08-16)


### Bug Fixes

* **ci:** stop publishing provenance to the registry ([42fd111](https://github.com/Garoze/Muninn/commit/42fd1113325c8ab33855cfdfb5cff690bf59b919))
* **ci:** verify each attestation with the tool that can read it ([6a5a9a8](https://github.com/Garoze/Muninn/commit/6a5a9a8afdde2c5d233364ccf9579ec1bdbefa39))
* **ci:** stop the provenance step destroying the SBOM attestation ([1a7c8e9](https://github.com/Garoze/Muninn/commit/1a7c8e97542be5a09fcf62a1e8cfe4d3800b587f))
* **ci:** keep the attestation verify from hanging the publish job ([21bbede](https://github.com/Garoze/Muninn/commit/21bbedef0d65d9e59e62b4b64e550cac9acca2d7))
* **ci:** take the digest to sign from ko's stdout ([30232eb](https://github.com/Garoze/Muninn/commit/30232eb94a12f8f8e3f6aa70c481619361c5a8f2))
* **ci:** build the image for every declared platform ([6ded274](https://github.com/Garoze/Muninn/commit/6ded274496192a5b479bbc7a16b9f5299a028a55))
* **ci:** keep develop's alphas numbered and :latest release-only ([512afd3](https://github.com/Garoze/Muninn/commit/512afd3c56d3da5bbe910692f9f60752a7609f7a))
* **ci:** stop seeding an untagged version into the manifest ([0a81955](https://github.com/Garoze/Muninn/commit/0a81955218e1b4d5e6236d1479e8bf9096407335))


### Features

* **ci:** attest the image SBOM and build provenance ([ce4651b](https://github.com/Garoze/Muninn/commit/ce4651b34bdf5f2969578df38208ef78c2e2214b))


### Code Refactoring

* **ci:** install the chart from the deploy targets ([b1b2bd7](https://github.com/Garoze/Muninn/commit/b1b2bd7c58ba8f336b83c16ef4ed99b4d09f6cb6))
* delete the static manifests the chart replaced ([2e25e49](https://github.com/Garoze/Muninn/commit/2e25e492bf23ef09034073b2971381e01923e7de))


### Documentation

* **testing:** correct what rendering the chart adds over its unit tests ([08adb85](https://github.com/Garoze/Muninn/commit/08adb858a55da8850a20c6a78aace96f52cbc882))

## [0.2.1](https://github.com/Garoze/Muninn/compare/v0.2.0...v0.2.1) (2026-08-15)


### Features

* **ci:** consolidate the changelog and back-merge main into develop ([85b222e](https://github.com/Garoze/Muninn/commit/85b222ea445c3eff5c312a65cfd47fcf65a91f07))


### Bug Fixes

* **docs:** correct CHANGELOG.md's version headings ([d8820f6](https://github.com/Garoze/Muninn/commit/d8820f65bbd74deeed5c7c9a552bdd072508fd37))
* **ci:** match linked changelog headings when renaming for a release ([805cc38](https://github.com/Garoze/Muninn/commit/805cc38e56e67a3d2cedba3aa9389f9f7cb7ab4e))

## 0.2.0 (2026-08-15)


### Features

* **chart:** add helm-unittest, kubeconform, and ct lint to CI ([dce9ad7](https://github.com/Garoze/Muninn/commit/dce9ad70393520b9bd2a4f70d330b6a995823297))
* **chart:** add opt-in cert-manager/CSI-driver dependencies and a webhook enable toggle ([0fbca89](https://github.com/Garoze/Muninn/commit/0fbca89e2f38a9026dbc8122e9dc5cc409a92456))
* **chart:** add the Helm chart's mechanical translation and core values ([cf493a6](https://github.com/Garoze/Muninn/commit/cf493a6928d238854f4b5a7ea3940920934bbc79))
* **ci:** add a nightly workflow installing the published chart and image ([b3b80bd](https://github.com/Garoze/Muninn/commit/b3b80bd227985cfaaa4f4e8d20ccf70e3f9a7c14))
* **ci:** publish and sign the Helm chart alongside the image ([4bb9d3b](https://github.com/Garoze/Muninn/commit/4bb9d3bd3add95018c0963c93e5129b784cf3f6a))
* **build:** add ko configuration ([46b56d9](https://github.com/Garoze/Muninn/commit/46b56d9f4b0131e579ff0ebecbd42f90085c025d))


### Bug Fixes

* **ci:** install helm explicitly before chart-testing-action ([5100e16](https://github.com/Garoze/Muninn/commit/5100e16a9966b5b84141c6b2a3b02033187a80ce))
* **ci:** register subchart repositories and fetch dependencies ([dcf6476](https://github.com/Garoze/Muninn/commit/dcf64766ec7c128a3b6c90cd0e1204139b7dce6e))
* **ci:** resync develop's release-please manifest after an official cut ([b1106e6](https://github.com/Garoze/Muninn/commit/b1106e6a2cb26e2fed90aaccb674f53dcb12b9aa))
* **config:** point the manager/webhook Deployments at the published image ([8edb69b](https://github.com/Garoze/Muninn/commit/8edb69b3494133a1fb1eae58b3a33023408aca63))


### Documentation

* **testing:** document the nightly tier ([d9b7274](https://github.com/Garoze/Muninn/commit/d9b727433ba2a21d85d0fe4da11a94bc2afb20dc))
* update build/deploy docs and troubleshooting for ko ([d56eedc](https://github.com/Garoze/Muninn/commit/d56eedce9ff895abca9f059057d334e5b6f9de4b))

## 0.1.0 (2026-08-15)

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
