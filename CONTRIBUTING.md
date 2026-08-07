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

| Package | Role |
|---|---|
| `internal/kube` | `ConfigSource` interface and `ConfigMapSource`, informers, patch-based cache sync |
| `internal/app` | domain layer: `Cache`, `DiscoveryService`, sentinel errors |
| `internal/transport/grpc` | proto and domain translation, gRPC handler, server, listener, TLS |
| `internal/webhook` | admission webhook: injection patch, secret references, `SecretProviderClass`, HTTPS server |
| `internal/discoveryclient` | shared gRPC dial helper |
| `internal/observability` | metrics, tracing, health |
| `internal/config` | env-driven configuration |

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

- Types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`
- Scopes: `api`, `app`, `kube`, `transport`, `config`, `ci`, `deps`

## Pull requests

Keep PRs small and focused on one change. If a change naturally splits
into independent concerns, open separate PRs rather than bundling them.

## Documenting a decision

If a change makes a real architectural tradeoff, a genuine alternative
was rejected, not only "the obvious way to do it": record it in
[`docs/design.md`](docs/design.md). If it's the kind of decision likely
to be questioned in review or revisited later, add a standalone entry
under [`docs/adr/`](docs/adr/) as well, following the existing records'
Context/Decision/Alternatives/Consequences shape.
