# ADR-0007: Namespace as an open-ended resolution scope, not an enforced tenant model

**Status:** Accepted

## Context

An earlier architecture modeled tenant identity as a first-class custom
resource, with its own lifecycle and provisioned-resource references,
and isolated tenant-owned data by giving each tenant its own namespace.
That model mirrored a real production control plane's reconciler output
without the reconciler behind it in this repository — there is no
provisioning system here that creates tenant identity, and no service
mesh here that validates caller identity at a namespace boundary. A
namespace boundary with neither of those behind it is an organizational
convention, not a security boundary, and modeling it as though it were
one overstated what this service can actually guarantee on its own.

## Decision

Treat namespace as an open-ended resolution scope a caller names in a
request, with no identity object, lifecycle, or validation behind it.
Multi-tenancy (one namespace per tenant, a ConfigMap per namespace) is a
usage pattern a consumer can adopt; this service does not encode,
validate, or enforce it.

## Alternatives considered

- **Keep a first-class tenant identity resource, generified.** Rejected:
  a generic "identity" resource with no reconciler or mesh behind it
  would still overstate what a namespace boundary actually provides on
  its own — the problem was the implied guarantee, not the resource's
  specific field names.

## Consequences

The API and domain layer are simpler: a namespace is a string a caller
provides, resolved directly against the cache, with no separate identity
lookup or validation step. The cost is that this service makes no claim
about tenant isolation beyond whatever the caller's own network access
and Kubernetes RBAC already provide — a consumer relying on this service
for tenant isolation without a service mesh or network policy enforcing
it at the namespace boundary has a gap this service does not close for
them, and this ADR states that gap explicitly rather than leaving it
implicit.
