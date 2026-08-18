# ADR-0008: Pluggable ConfigSource interface instead of a fixed set of custom resources

**Status:** Accepted

## Context

Muninn makes no assumptions about what platform or infrastructure a
consumer runs downstream, or how a consumer organizes its own
configuration data. Committing to a specific, fixed custom resource
shape, requiring a CRD installation and a predetermined schema, would encode
exactly that kind of assumption, and would have to be rebuilt or extended
whenever a consumer's data does not fit the chosen shape.

## Decision

Define a pluggable source abstraction, covering what to watch, how to
scope it, and how to extract the fields it contributes to a cache entry,
and write the watch layer, the cache, and the domain layer against that
abstraction rather than against any specific resource type. A source
watching core `ConfigMap` objects, scoped by a configurable label
selector, is registered as the default and only source today, chosen
because a ConfigMap requires no CRD installation and its `data` field is
open-ended. A second source kind, such as a bring-your-own custom
resource, registers by satisfying the same abstraction, with no change to
the watch layer, the cache, or the domain layer.

## Alternatives considered

- **Keep a fixed set of custom resources, generified in name only.**
  Rejected: still requires a CRD installation step and a fixed schema per
  resource type, and still commits Muninn to a specific shape of
  configuration rather than deferring that to whatever object a consumer
  already has.
- **Support only ConfigMaps, with no pluggable interface at all.**
  Rejected: matches today's single registered source, but forecloses a
  bring-your-own-CRD integration path without a rewrite of the watch
  layer, the cache's per-source keying, and the domain layer's
  source-shape reporting.
- **Key the cache's patch-merge purely by source type.** Rejected: a type
  identifier alone cannot distinguish two independently registered
  sources of the same type: for example, two sources both watching
  ConfigMaps but scoped by different label selectors to express distinct
  configuration layers in one namespace. Keying by type would let the
  second such registration overwrite the first's contribution on any
  object-name collision without reporting it. The abstraction therefore
  separates a source's externally reported type identity, used for
  `Describe` and for selecting which registered sources are active, from
  a second, cache-facing identity that each registration controls
  independently and that defaults to the type identity when a source has
  no sibling to distinguish itself from.

## Consequences

The watch layer's upsert and delete handling generalizes to any object
implementing the interface. The cache's patch-merge keying is
per-registration rather than purely per-type, so two sources of the same
type stay distinguishable as long as each registration sets its own
cache-facing identity. The default falls back to the type identity, which
is correct precisely when only one source of that type is registered.
A registration mistake here would otherwise cause ongoing data loss,
discovered only by noticing absent configuration, so the watch layer
validates at startup that every registered source's cache-facing identity
is distinct and fails construction on a collision.

Distinct keying is necessary but not sufficient for two sources of one
type. A watch implementation that indexes its informers by object type
would hand both registrations a single informer and a single scoping
selector, discarding the other without reporting it, leaving both sources
correctly keyed but carrying identical data. Each registration therefore
gets its own informer, scoped by its own selector. The cost is one watch
and one in-memory copy per registration rather than per type, so two
sources whose selectors overlap hold the objects they both match twice.
A test covers this by registering two sources of the same type against a
real API server and asserting each observes only its own selector; a fake
source cannot exercise it, since the failure is in informer construction
rather than in the merge.

The API's `Describe` operation reports the active source's shape (kind,
label selector, scope) rather than an enumerated key list, since there
is no fixed vocabulary to enumerate once the underlying source is
open-ended. The cost is one more layer of indirection between the watch
layer and any specific Kubernetes type it watches, paid once, in exchange
for the CRD-watching case requiring no change to code outside the new
source's own registration.
