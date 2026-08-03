# ADR-0008: Explicit key whitelist instead of reflection-based field exposure

**Status:** Accepted

## Context

Without an explicit contract, the API's effective surface is "whatever
fields the underlying custom resources currently happen to have" —
anything a consumer can currently read becomes something they can
depend on, whether or not that was intended.

## Decision

Only keys present in an explicit, documented whitelist are queryable;
anything outside that set is rejected with a precise error rather than
silently omitted or passed through.

## Alternatives considered

- **Expose resource fields directly, by reflection or pass-through.**
  Rejected: couples the API contract to internal resource shape,
  breaking consumers on any schema change to those resources.

## Consequences

Every field added to the underlying resources needs an explicit
whitelist update before it becomes queryable — a deliberate hurdle,
accepted in exchange for an API contract that doesn't silently shift
under consumers as internal resource shapes evolve.
