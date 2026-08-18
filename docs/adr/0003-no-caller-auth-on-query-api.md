# ADR-0003: No caller authentication or authorization on the Query API

**Status:** Accepted

## Context

The data Muninn serves is configuration a consumer's own workload reads
to change its own runtime behavior. It is not credential material, and it
does not itself grant access to anything else. Whatever downstream
authorization a consumer performs with that data is the consumer's
concern rather than something Muninn mediates.

## Decision

The gRPC API performs no authentication or authorization of its own
callers.

## Alternatives considered

- **A gRPC authentication interceptor validating caller identity.** Not
  implemented: judged an orthogonal concern to configuration resolution,
  and one a real deployment would more naturally solve at a
  network-policy or service-mesh layer than reimplement inside Muninn.

## Consequences

As built, network reachability is the only access control on this API.
That includes the shared dial helper used by the debugging CLI and by the
injected init container and sidecar, which default to plaintext for the
same reason and connect over TLS only when a deployment opts the server
into it, matching the server's own posture.

This is a documented limitation rather than an oversight. A production
deployment of this pattern would need cluster-internal network policy, or
mutual TLS between Muninn and its callers, to make the boundary
enforceable. The decision holds only for as long as nothing flowing
through the API grants access to anything else, which is why secret
material is kept out of it entirely (see
[ADR-0012](0012-csi-secret-delivery.md)).
