# ADR-0008: Pluggable ConfigSource interface instead of a fixed set of custom resources

**Status:** Accepted

## Context

An earlier architecture defined three fixed custom resource types
(identity, runtime configuration, policy), each requiring a CRD
installation and each shaped like the output of a real production
control plane's reconciler — identity provisioning, service-mesh JWT
validation, authorization binding sync — with no reconciler behind them
in this repository. That shape assumed a platform this repository
doesn't have. This service does not know what a tenant is, what a
policy is, or what identity or mesh infrastructure exists downstream of
it; it resolves configuration from whatever Kubernetes objects a
consumer points it at.

## Decision

Define a pluggable source abstraction — covering what to watch, how to
scope it, and how to extract the fields it contributes to a cache
entry — that the watch layer, the cache, and the domain layer are written
against, rather than against any specific resource type. A source
watching core `ConfigMap` objects, scoped by a configurable label
selector, is registered as the default and only source today — chosen
because a ConfigMap requires no CRD installation and is inherently
open-ended in its `data` shape. A second source kind (a bring-your-own
custom resource, for example) registers by satisfying the same
abstraction, with no change to the watch layer, the cache, or the domain
layer.

## Alternatives considered

- **Keep a fixed set of custom resources, generified in name only.**
  Rejected: still requires a CRD installation step and a fixed schema
  per resource type, and still commits this service to a specific shape
  of "what configuration looks like" rather than deferring that entirely
  to whatever object a consumer already has.
- **Support only ConfigMaps, with no pluggable interface at all.**
  Rejected: correctly matches today's single registered source, but
  forecloses a bring-your-own-CRD integration path without a rewrite of
  the watch layer, the cache's per-source keying, and the domain layer's
  source-shape reporting.

## Consequences

The watch layer's upsert/delete handling and the cache's patch-merge
keying (by source kind and object name, not by a specific resource type)
generalize cleanly to any object implementing the interface, proven by a
minimal fake source used in tests rather than a second production
implementation. The API's `Describe` operation reports the active
source's shape (kind, label selector, scope) rather than an enumerated
key list, since there is no fixed vocabulary to enumerate once the
underlying source is open-ended. The cost is one more layer of
indirection between the watch layer and any specific Kubernetes type it
watches, paid once, in exchange for the CRD-watching case requiring no
change to code outside the new source's own registration.
