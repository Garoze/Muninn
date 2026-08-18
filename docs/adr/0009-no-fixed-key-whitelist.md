# ADR-0009: No fixed key whitelist, resolving directly against source data

**Status:** Accepted

## Context

Muninn's underlying source data is arbitrary key-value pairs rather than
a fixed schema: a ConfigMap's `data` field, or the fields of any other
registered source (see
[ADR-0008](0008-pluggable-config-source.md)). An explicit,
hand-maintained whitelist of valid keys would have nothing stable to
validate against, since the set of valid keys is whatever the
currently-watched source objects contain, and that set changes
independently of Muninn.

## Decision

Resolve each requested key by direct lookup against the scope's merged
source data. A key not present in that data is reported back in a
`missing_keys` list on the response, rather than rejected with an
`InvalidArgument` error.

## Alternatives considered

- **Keep a whitelist, populated dynamically from currently-observed
  keys.** Rejected: a key legitimately absent at a given moment, such as a
  ConfigMap briefly missing a field during a rollout, would be
  indistinguishable from a key that was never valid, producing the same
  ambiguity a whitelist exists to remove. The dynamic whitelist would also
  have to be correct to be useful, which requires the same direct lookup
  this decision performs.

## Consequences

There is no fixed vocabulary for a caller to violate, so `Query` cannot
reject a request on the grounds that a key is not part of the schema; it
can only report that a specific key is not present at the time of the
request. The `Describe` operation's contract follows from this: it
reports the active source's shape (kind, label selector, scope) rather
than an enumerated key list, since there is no such list to enumerate.
The tradeoff is the loss of a guardrail against querying a key that was
never intended to exist. That guardrail lives entirely in whatever
contract a consumer and the operator populating its ConfigMap agree on
out of band, not in anything Muninn validates.
