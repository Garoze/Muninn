# ADR-0004: Three separate custom resources instead of one combined resource

**Status:** Accepted

## Context

Tenant identity, runtime configuration, and policy have different
owners and different change cadences in a real deployment — identity
and provisioned resource references change when infrastructure is
provisioned, configuration changes when operators adjust runtime
behavior, and policy changes when security rules are updated.

## Decision

Model these as three separate custom resource types, rather than one
resource with three sections.

## Alternatives considered

- **One combined resource covering all three concerns.** Rejected:
  would need either a single writer for the entire object, or a
  subresource/merge strategy to let different actors safely write
  different parts of it — coordination Kubernetes already provides for
  free across three separate resources.

## Consequences

Three independent watches and the patch-merge logic in ADR-0001 exist
specifically to keep a coherent per-tenant view across three resources,
instead of one watch with no merge step needed. That added complexity
is the direct price of letting each concern be owned and written
independently by whichever actor is responsible for it.
