# Architecture Decision Records

Short, standalone records for the decisions with the most significant
tradeoffs — the ones most likely to be questioned in review, or
revisited later. Broader design rationale, including decisions not
listed here, lives in [`../design.md`](../design.md).

| ADR | Decision |
|---|---|
| [0001](0001-patch-based-cache-merge.md) | Patch-based cache merge across independently-owned resources |
| [0002](0002-cluster-scoped-rbac-for-dynamic-namespaces.md) | Cluster-scoped RBAC for a dynamic, per-tenant namespace set |
| [0003](0003-namespace-per-tenant-isolation.md) | Namespace-per-tenant isolation model |
| [0004](0004-three-separate-custom-resources.md) | Three separate custom resources instead of one combined resource |
| [0005](0005-no-caller-auth-on-query-api.md) | No caller authentication or authorization on the Query API |
| [0006](0006-grpc-over-rest.md) | gRPC over REST as the transport protocol |
| [0007](0007-concrete-domain-service-no-interface.md) | Concrete domain service dependency instead of a handler-defined interface |
| [0008](0008-explicit-key-whitelist.md) | Explicit key whitelist instead of reflection-based field exposure |
| [0009](0009-no-client-library.md) | Decision not to build a client library |
