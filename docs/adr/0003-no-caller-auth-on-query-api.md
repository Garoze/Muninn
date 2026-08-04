# ADR-0003: No caller authentication or authorization on the Query API

**Status:** Accepted

## Context

The data this service serves is configuration a consumer's own workload
reads to change its own runtime behavior — not credentials, and not data
that itself grants access to anything else. Whatever downstream
authorization a consumer performs with that data is the consumer's own
concern, not something this service mediates.

## Decision

The gRPC API performs no authentication or authorization of its own
callers.

## Alternatives considered

- **A gRPC authentication interceptor validating caller identity.** Not
  implemented: judged an orthogonal concern from configuration
  resolution, and one a real deployment would more naturally solve at a
  network-policy or service-mesh layer than reimplement inside this
  service.

## Consequences

As built, network reachability is the only access control on this API —
including for the shared gRPC dial helper used by both the debugging CLI
and the admission webhook's injected init container/sidecar, which
connect over plaintext for the same reason. This is a stated limitation,
not an oversight — a genuine production deployment of this pattern would
need to sit behind cluster-internal network policy or mutual TLS between
this service and its callers. Of all the decisions recorded in this
project, this is the one most likely to be challenged in review, which is
exactly why it's recorded on its own rather than as an aside in a longer
document.
