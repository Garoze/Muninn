# ADR-0004: gRPC over REST as the transport protocol

**Status:** Accepted

## Context

Consumers of this API are other backend services within the same
cluster, not browsers or external third parties.

## Decision

Expose the API over gRPC, with a query operation and a schema-discovery
operation, rather than a REST/JSON interface.

## Alternatives considered

- **REST/JSON.** Rejected: would need a separately hand-built and
  hand-maintained schema-discovery mechanism, where gRPC reflection
  plus a discovery RPC gives that for free. Offers no benefit over gRPC
  for an internal, service-to-service contract.

## Consequences

Heavier client tooling than plain HTTP for any consumer outside the
cluster's own service mesh, an accepted cost given the API's actual
consumer population. This also locks the API to gRPC-capable clients
going forward; a future external-facing use case would need a separate
gateway rather than a protocol change to this service itself.
