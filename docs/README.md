# Documentation

- [`deployment.md`](deployment.md): running the resolver and the admission
  webhook in a cluster, and what each one requires.
- [`api.md`](api.md): the gRPC surface, `muninnctl`, and calling it with
  `grpcurl`.
- [`secret-references.md`](secret-references.md): the secret reference
  convention and how CSI delivers the values Muninn never holds.
- [`config-sources.md`](config-sources.md): writing and registering a
  bring-your-own configuration source.
- [`configuration.md`](configuration.md): every environment variable, its
  default, and what it affects.
- [`observability.md`](observability.md): metrics, tracing and health
  endpoints, with a local Jaeger walkthrough.
- [`testing.md`](testing.md): the test tiers, what each verifies, and how to
  run them.
- [`design.md`](design.md): architecture and design rationale covering the
  problem being solved, goals and non-goals, and the reasoning behind every
  significant decision.
- [`adr/`](adr/): standalone Architecture Decision Records for the decisions
  with the most significant tradeoffs.
- [`troubleshooting.md`](troubleshooting.md): known issues encountered running
  Muninn locally or in a cluster, and how to recognize them.

The [README](../README.md) covers what Muninn does and how to run it.
Start there first.
