# ADR-0003: Namespace-per-tenant isolation model

**Status:** Accepted

## Context

Tenant data needs an isolation boundary that composes with Kubernetes'
own access-control and network primitives, rather than one this service
has to enforce itself in application logic.

## Decision

Each tenant's namespace-scoped resources live in a namespace created
specifically for that tenant, rather than a shared namespace
distinguished by labels.

## Alternatives considered

- **A shared namespace with tenant-identifying labels.** Rejected: every
  consumer of these resources — including RBAC and network policy —
  would need label-selector filtering to reconstruct the isolation a
  namespace boundary already provides natively.

## Consequences

Isolation is stronger and composes for free with existing Kubernetes
primitives (RBAC, network policy) without this service having to
enforce tenant boundaries itself. The price is paid one level up: the
namespace set is open-ended and grows with tenant count, which is what
forces the cluster-scoped RBAC decision in ADR-0002 — a direct
structural consequence of this choice, not an independent cost.
