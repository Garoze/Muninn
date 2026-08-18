# ADR-0006: No client library shipped in this repository

**Status:** Accepted

## Context

A client consuming Muninn's data would typically merge it through a
layered, pluggable loader. That is a different engineering problem from
watching and caching Kubernetes objects, which is what this repository
exists to demonstrate.

## Decision

No client library or configuration-loading SDK ships in this repository.

## Alternatives considered

- **Ship a thin client library alongside Muninn.** Rejected: it addresses
  a distinct problem from this repository's own scope, and bundling it
  here would combine two separable concerns in one project rather than
  develop either.

## Consequences

Consumers calling the gRPC API directly write their own client calls,
with no purpose-built client to import. Consumers who do not want to
write gRPC client code have a delivery path instead of a client library:
a mutating admission webhook injects a container that resolves
configuration and writes it to a file mounted into the consumer's own
container. That path is the concrete alternative this decision depends
on; without it, declining to ship a client would leave direct gRPC calls
as the only integration option.

If a general-purpose layered-loader pattern is ever built as a standalone
artifact, it belongs in its own generic, non-Kubernetes-specific
repository. That is a deliberate boundary rather than an unfinished piece
of this repository.
