# ADR-0011: A dedicated Resolve RPC instead of overloading Query

**Status:** Accepted

## Context

The admission webhook's injected init container and sidecar need to
write out *everything* currently resolved for a scope, with no keys
enumerated up front, a different shape of request than `Query`, which
is built for a caller (a human via a debugging CLI, or a service that
knows exactly which keys it wants) who names specific keys.

## Decision

Add a dedicated `Resolve` operation (`Resolve(namespace) →
map<string, Value>`, alongside a revision marker) rather than overloading
`Query` so that an empty requested-keys list means "return everything."

## Alternatives considered

- **Overload `Query` with an empty key list meaning "all keys."**
  Rejected: an empty list on `Query` is ambiguous between a caller bug
  (nothing was actually requested) and an intentional wildcard, and
  resolving that ambiguity in favor of the wildcard reading would muddy
  `Query`'s otherwise unambiguous keys-in/keys-out contract for every
  other caller of it.

## Consequences

`Query` and `Resolve` currently duplicate the same cache-lookup
precedence logic (readiness check, scope lookup, staleness check) rather
than sharing a helper. An extraction was considered and deliberately not
made, since the two operations' contracts are different enough (specific
keys with a `missing_keys` list, versus everything with a revision
marker) that a shared helper would need to serve both shapes rather than
cleanly serve either. The webhook's injected containers, and the
resolve-mode CLI they invoke, call `Resolve` exclusively; the
human-facing debugging CLI is untouched by this and stays scoped to
`Query`/`Describe`.
