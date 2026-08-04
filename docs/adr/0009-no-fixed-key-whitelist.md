# ADR-0009: No fixed key whitelist — resolve directly against source data

**Status:** Accepted

## Context

An earlier architecture validated every requested key against an
explicit, hand-maintained whitelist, because the underlying data came
from a fixed set of custom resources with a correspondingly fixed,
enumerable field set. Once the underlying source became an open-ended,
pluggable set of Kubernetes objects (see
[ADR-0008](0008-pluggable-config-source.md)) — a ConfigMap's `data` field
is inherently arbitrary key-value pairs, not a fixed schema — a
whitelist would have nothing stable to validate against, since the set
of valid keys is now whatever the currently-watched source objects
happen to contain.

## Decision

Resolve each requested key by direct lookup against the scope's merged
source data. A key not present in that data is reported back in a
`missing_keys` list on the response, rather than rejected with an
`InvalidArgument` error.

## Alternatives considered

- **Keep a whitelist, populated dynamically from currently-observed
  keys.** Rejected: a key legitimately absent right now (a ConfigMap
  briefly missing a field during a rollout, for example) would be
  indistinguishable from a key that was never valid, producing the exact
  ambiguity a whitelist exists to remove — the dynamic whitelist would
  itself need to be correct to be useful, which requires the same direct
  lookup this decision performs anyway.

## Consequences

There is no fixed vocabulary for a caller to violate, so `Query` cannot
reject a request for being "not a real key" — only report that a
specific key isn't present right now. The `Describe` operation's
contract shifts accordingly: it reports the active source's shape (kind,
label selector, scope), not an enumerated key list, since there is no
such list to enumerate. The cost of the earlier model — every new field
needing an explicit whitelist update before it became queryable — is
gone, at the cost of losing a compile-time-adjacent guardrail against
querying a key that was never meant to exist; that guardrail now lives
entirely in whatever contract a consumer and the operator populating
its ConfigMap agree on out of band.
