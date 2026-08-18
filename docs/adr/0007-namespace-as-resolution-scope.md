# ADR-0007: Namespace as an open-ended resolution scope, not an enforced tenant model

**Status:** Accepted

## Context

Namespace is Muninn's only resolution scope. A first-class tenant
concept, with its own identity object, lifecycle, and validation layered
on top of that scope, is a reasonable thing a consumer might want.
Building one here would imply a security guarantee, validated and
isolated tenant identity, that Muninn cannot provide on its own. There is no
provisioning system behind it that creates tenant identity, and no
service mesh behind it that validates caller identity at a namespace
boundary. A namespace boundary without either of those is an
organizational convention, not a security boundary.

## Decision

Treat namespace as an open-ended resolution scope a caller names in a
request, with no identity object, lifecycle, or validation behind it.
Multi-tenancy (one namespace per tenant, a ConfigMap per namespace) is
a usage pattern a consumer can adopt; Muninn does not encode, validate,
or enforce it.

## Alternatives considered

- **Keep a first-class tenant identity resource, generified.** Rejected:
  a generic identity resource with no reconciler or service mesh behind
  it would still overstate what a namespace boundary provides on its own.
  The problem was the implied guarantee, not the resource's specific
  field names.

## Consequences

The API and domain layer carry no tenant model: a namespace is a string a
caller provides, resolved directly against the cache, with no separate
identity lookup or validation step. The cost is that Muninn makes no
claim about tenant isolation beyond whatever the caller's own network
access and Kubernetes RBAC already provide. A consumer relying on Muninn
for tenant isolation, without a service mesh or network policy enforcing
it at the namespace boundary, has a gap Muninn does not close, and this
record states that gap explicitly rather than leaving it implicit.

A multi-tenant deployment (one namespace per tenant, a labeled ConfigMap
in each) composes with this decision, since it requires nothing from
Muninn beyond what any other multi-namespace usage already receives: the
cache's namespace-keyed isolation is unit-tested against multiple
distinct namespaces. The README's walkthrough uses a single namespace,
since the multi-tenant arrangement is a consumer's convention rather than
behavior Muninn implements.
