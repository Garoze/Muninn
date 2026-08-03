# ADR-0009: Decision not to build a client library

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

Consumers write their own gRPC calls against this service today, with
no purpose-built client to import. If this pattern is ever built as a
standalone artifact, it belongs in its own generic,
non-Kubernetes-specific repository — a deliberate, stated boundary
rather than an unfinished piece of this one.
