# ADR-0004: gRPC over REST as the transport protocol

**Status:** Accepted

## Context

Consumers of this API are other backend services within the same cluster,
not browsers or external third parties.

## Decision

Expose the API over gRPC, with a keys-in/keys-out query operation, a
whole-scope resolve operation, and an operation reporting the active
source's shape, rather than a REST/JSON interface.

## Alternatives considered

- **REST/JSON.** Rejected: the service contract would have to be
  published and maintained by hand, where gRPC reflection exposes it as
  part of the protocol, leaving the source-shape operation to report only
  what reflection cannot describe. Its advantages, broader client support
  and inspectability with generic HTTP tooling, apply to consumers outside
  the cluster, which this API does not have.

## Consequences

Client tooling is heavier than plain HTTP for any consumer outside the
cluster's own service mesh, an accepted cost given the API's actual
consumer population. The API is also committed to gRPC-capable clients
going forward; a future external-facing use case would need a separate
gateway rather than a protocol change to Muninn itself.
