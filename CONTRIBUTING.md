# Contributing to Muninn

Muninn is a single-maintainer portfolio/reference project. Issues and
pull requests are welcome regardless: this guide documents the same
workflow the maintainer follows. By participating, you agree to abide by
the [Code of Conduct](./CODE_OF_CONDUCT.md).

## Getting started

Follow the [README's "Getting started"](./README.md#getting-started)
section: Go 1.26+, a Kubernetes cluster, and optionally `grpcurl`. You
should be able to `make sample` and `make run` successfully before making
changes.

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
shape and coverage level.

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
