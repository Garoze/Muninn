# ADR-0005: No caller authentication or authorization on the Query API

**Status:** Accepted

## Context

The Policy resource carries JWT validation rules and role bindings —
data describing how *downstream consumers* should authorize their own
traffic, not credentials for calling this service itself.

## Decision

The gRPC API performs no authentication or authorization of its own
callers.

## Alternatives considered

- **A gRPC authentication interceptor validating caller identity.** Not
  implemented: judged an orthogonal concern from tenant-data
  projection, and one a real deployment would more naturally solve at a
  network-policy or service-mesh layer than reimplement inside this
  service.

## Consequences

As built, network reachability is the only access control on this API.
This is a stated limitation, not an oversight — a genuine production
deployment of this pattern would need to sit behind cluster-internal
network policy or mutual TLS between this service and its callers. Of
all the decisions recorded in this project, this is the one most likely
to be challenged in review, which is exactly why it's recorded on its
own rather than as an aside in a longer document.
