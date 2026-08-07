# Architecture Decision Records

Short, standalone records for the decisions with the most significant
tradeoffs: the ones most likely to be questioned in review, or
revisited later. Broader design rationale, including decisions not
listed here, lives in [`../design.md`](../design.md).

| ADR | Decision |
|---|---|
| [0001](0001-patch-based-cache-merge.md) | Patch-based cache merge across independently-owned source objects |
| [0002](0002-cluster-scoped-rbac-for-dynamic-namespaces.md) | Cluster-scoped RBAC for a dynamic namespace set |
| [0003](0003-no-caller-auth-on-query-api.md) | No caller authentication or authorization on the Query API |
| [0004](0004-grpc-over-rest.md) | gRPC over REST as the transport protocol |
| [0005](0005-concrete-domain-service-no-interface.md) | Concrete domain service dependency instead of a handler-defined interface |
| [0006](0006-no-client-library.md) | Decision not to build a client library |
| [0007](0007-namespace-as-resolution-scope.md) | Namespace as an open-ended resolution scope, not an enforced tenant model |
| [0008](0008-pluggable-config-source.md) | Pluggable ConfigSource interface instead of a fixed set of custom resources |
| [0009](0009-no-fixed-key-whitelist.md) | No fixed key whitelist, resolving directly against source data |
| [0010](0010-single-process-webhook.md) | Webhook runs as a subcommand of the same binary, deployed as a separate process |
| [0011](0011-resolve-rpc.md) | A dedicated Resolve RPC instead of overloading Query |
| [0012](0012-csi-secret-delivery.md) | Secrets delivered via CSI, never through Muninn's own process |
