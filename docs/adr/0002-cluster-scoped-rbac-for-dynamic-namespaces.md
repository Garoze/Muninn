# ADR-0002: Cluster-scoped RBAC for a dynamic, per-tenant namespace set

**Status:** Accepted

## Context

Two of the three watched resource types are namespace-scoped, but they
live across a namespace created per tenant — an open-ended set that
grows at runtime, not a fixed list known in advance.

## Decision

Grant access via a single cluster-scoped role bound cluster-wide, rather
than a namespace-scoped role bound per tenant namespace.

## Alternatives considered

- **A namespaced role created alongside each tenant namespace.**
  Rejected: couples this service's access provisioning to tenant
  provisioning, and still can't grant access to a namespace before it
  exists.

## Consequences

The access grant is broader than any single tenant's resources
require — a real, accepted widening of blast radius in exchange for
correctly expressing "watch this resource type across a namespace set
that doesn't exist yet." The grant stays narrow along other axes
(read-only, no subresources) to partially offset this. This binding
shape is a direct structural consequence of the tenant isolation model
(see ADR-0003) — the namespace set only grows because tenants are
isolated by namespace in the first place.
