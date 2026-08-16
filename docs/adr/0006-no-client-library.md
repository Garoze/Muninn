# ADR-0006: Decision not to build a client library

**Status:** Accepted

## Context

A client consuming this service's data would typically merge it through
a layered, pluggable loader, a pattern that solves a different
engineering problem than watching and caching Kubernetes objects, which
is what this repository exists to demonstrate.

## Decision

No client library or configuration-loading SDK ships in this
repository.

## Alternatives considered

- **Ship a thin client library alongside this service.** Rejected: it
  addresses a distinct problem from this repository's own scope, and
  bundling it here would blur two separable concerns into one project
  rather than sharpen either.

## Consequences

Consumers calling the gRPC API directly write their own client calls,
with no purpose-built client to import. Consumers who would rather avoid
that entirely have a delivered path instead of a client library: a
mutating admission webhook that injects a container which resolves
configuration and writes it to a file, mounted into the consumer's own
container. That path is the concrete alternative this decision depends
on; without it, declining to ship a client would leave direct gRPC calls
as the only integration option.

If a general-purpose layered-loader pattern is ever built as a standalone
artifact, it belongs in its own generic, non-Kubernetes-specific
repository, a deliberate boundary rather than an unfinished piece of
this one.
