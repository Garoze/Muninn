# ADR-0011: A dedicated Resolve RPC instead of overloading Query

**Status:** Accepted

## Context

The admission webhook's injected init container and sidecar need to write
out everything currently resolved for a scope, with no keys enumerated up
front. That is a different shape of request from `Query`, which is built
for a caller that names specific keys: an operator using the debugging
CLI, or a service that knows exactly which keys it wants.

## Decision

Add a dedicated `Resolve` operation (`Resolve(namespace) →
map<string, Value>`, alongside a revision marker) rather than overloading
`Query` so that an empty requested-keys list means "return everything."

## Alternatives considered

- **Overload `Query` with an empty key list meaning all keys.** Rejected:
  an empty list on `Query` is ambiguous between a caller defect, where
  nothing was actually requested, and an intentional wildcard. Resolving
  that ambiguity in favor of the wildcard reading would weaken `Query`'s
  otherwise unambiguous keys-in/keys-out contract for every other caller.

## Consequences

`Query` and `Resolve` duplicate the same cache-lookup precedence logic:
readiness check, scope lookup, staleness check. An extraction was
considered and deliberately not made, since the two operations' contracts
differ enough (specific keys with a `missing_keys` list, versus
everything with a revision marker) that a shared helper would have to
serve both shapes rather than serve either directly. The webhook's
injected containers, and the resolve-mode CLI they invoke, call `Resolve`
exclusively; the debugging CLI is unaffected and remains scoped to
`Query` and `Describe`.
