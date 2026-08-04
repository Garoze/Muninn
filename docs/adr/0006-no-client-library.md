# ADR-0006: Decision not to build a client library

**Status:** Accepted

## Context

A client consuming this service's data would typically merge it through
a layered, pluggable loader — a pattern that solves a different
engineering problem than watching and caching Kubernetes custom
resources, which is what this repository exists to demonstrate.

## Decision

No client library or configuration-loading SDK ships in this
repository.

## Alternatives considered

- **Ship a thin client library alongside this service.** Rejected: it
  addresses a distinct problem from this repository's own scope, and
  bundling it here would blur two separable concerns into one project
  rather than sharpen either.

## Consequences

Consumers calling the gRPC API directly write their own client calls
today, with no purpose-built client to import. Consumers who would
rather avoid that entirely have a second, delivered path instead of a
client library: a mutating admission webhook that injects a container
which resolves configuration and writes it to a file, mounted into the
consumer's own container. That path did not exist when this decision was
first made; its later addition is the concrete alternative this decision
anticipated, not a reversal of it. If a general-purpose layered-loader
pattern is ever built as a standalone artifact, it still belongs in its
own generic, non-Kubernetes-specific repository — a deliberate, stated
boundary rather than an unfinished piece of this one.
