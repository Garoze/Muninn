# ADR-0009: Decision not to build a client library

**Status:** Accepted

## Context

The layered-configuration-merge pattern this service's data would
typically feed into — a pluggable, layered loader on the consuming
side — already exists as separately-built code against a real
production system.

## Decision

No client library or configuration-loading SDK ships in this
repository.

## Alternatives considered

- **Ship a thin client library alongside this service.** Rejected for
  this repository's scope specifically: the engineering problem it
  would demonstrate is already demonstrated elsewhere, and bundling it
  here would blur two distinct pieces of work into one rather than add
  new signal.

## Consequences

Consumers write their own gRPC calls against this service today, with
no purpose-built client to import. If this pattern is ever built as a
standalone artifact, it belongs in its own generic,
non-Kubernetes-specific repository — a deliberate, stated boundary
rather than an unfinished piece of this one.
