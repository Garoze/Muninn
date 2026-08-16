# Contributing to Muninn

Issues and pull requests are welcome. By participating, you agree to abide by
the [Code of Conduct](./CODE_OF_CONDUCT.md).

The current implementation covers the scope this project set out to
demonstrate, so a large feature addition is worth raising in an issue before
writing it: some are a better fit for a fork than for a change here, and
that is easier to establish before the work than after it.

Bug reports are always welcome, particularly from anyone running Muninn
against a real cluster, and are investigated on a best-effort basis. So are
fixes, documentation corrections, and test coverage for behavior that already
exists.

## Getting started

Follow the [README's "Quick start"](./README.md#quick-start)
section: Go 1.26+, a Kubernetes cluster, and optionally `grpcurl`. You
should be able to `make sample` and `make run` successfully before making
changes.

## Project layout

| Path | Role |
|---|---|
| `internal/kube` | `ConfigSource` interface and `ConfigMapSource`, informers, patch-based cache sync |
| `internal/app` | domain layer: `Cache`, `DiscoveryService`, sentinel errors |
| `internal/transport/grpc` | proto and domain translation, gRPC handler, server, listener, TLS |
| `internal/webhook` | admission webhook: injection patch, secret references, `SecretProviderClass`, HTTPS server |
| `internal/discoveryclient` | shared gRPC dial helper |
| `internal/observability` | metrics, tracing, health |
| `internal/config` | env-driven configuration |
| `charts/muninn` | the chart, which is the only install path |
| `test/` | integration (envtest) and end-to-end tiers |
| `.github/workflows` | CI, publishing, the release cut, the back-merge, and the nightly |
| `hack/` | scripts the release workflows call, with their fixtures |

The chart's `README.md` is generated from `values.yaml`'s comments by
`make chart-docs`, and CI fails if it is out of date. Document a value by
commenting it in `values.yaml`, never by editing the README.

The domain layer imports no Kubernetes or gRPC types, and a change that
introduces one there belongs at an edge instead. [`docs/design.md`](docs/design.md)
covers why the boundary sits where it does.

## Making changes

```bash
make fmt          # gofmt -l -w .
make vet          # go vet ./...
make lint         # golangci-lint run ./...
make test-unit    # go test ./... -short
```

All four must pass before opening a PR; CI runs the same checks.

If your change touches `proto/v1/discovery.proto`, also run:

```bash
make proto    # regenerate gRPC stubs (requires protoc)
```

and confirm the regenerated code in `gen/discovery/v1` is committed
alongside the `.proto` change.

## Tests

Add tests for new behavior, including negative/error cases alongside the
happy path. See `internal/` for the existing test files for the expected
shape and coverage level, and [`docs/testing.md`](docs/testing.md) for what
each tier is responsible for proving.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):
`type(scope): description`

- Types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `ci`
- Scopes: `api`, `app`, `kube`, `transport`, `config`, `ci`, `deps`

The test a type has to pass is whether the commit can change what gets
published. `feat`, `fix` and `refactor` can, and are visible in the
changelog; a refactor alters the artifact even when behaviour is
unchanged. `docs`, `test`, `ci` and `chore` cannot: documentation ships in
this repository rather than inside the image or the chart, so a
documentation change leaves both byte-identical. Those are hidden, and
pipeline plumbing belongs under `ci` or `chore` rather than under `fix`.

## Pull requests

Keep PRs small and focused on one change. If a change naturally splits
into independent concerns, open separate PRs rather than bundling them.

## Releases

Nothing is released by hand, and every step below publishes through the
same pipeline under the same identity — see
[`docs/verification.md`](docs/verification.md) for what that identity is
and how a consumer checks it.

**Prereleases happen on their own.** Merging to `develop` updates a
release pull request holding the next version and its changelog entry.
Merging *that* tags the prerelease, which publishes the image, the chart
and the attestations. Nothing about this is optional: exercising the
release path continuously is what stops its defects from being discovered
during a release.

The first prerelease after a release carries no increment, and the ones
after it do. Both shapes are prereleases and neither ever receives the
floating `latest` tag, which is reserved for an exact three-part version.

**Official releases are cut deliberately**, by dispatching the release
workflow from `main` with the version to cut. It consolidates the
prerelease changelog entries into one, tags, publishes, and merges `main`
back into `develop`.

That last step matters more than it looks. A `develop` → `main` pull
request only advances `main`; `develop` never moves, so anything
committed on `main` — a release's tag and changelog, or a fix applied
there directly — stays outside `develop`'s history until something merges
in the other direction. A push to `main` now opens that merge
automatically, and both routes exit cleanly when there is nothing to
bring back.

**Dependency updates target `develop`**, not the default branch, so the
workflows a bump changes are exercised by CI and a prerelease before they
can reach a release.

## Documenting a decision

If a change makes a real architectural tradeoff, a genuine alternative
was rejected, not only "the obvious way to do it": record it in
[`docs/design.md`](docs/design.md). If it's the kind of decision likely
to be questioned in review or revisited later, add a standalone entry
under [`docs/adr/`](docs/adr/) as well, following the existing records'
Context/Decision/Alternatives/Consequences shape.
