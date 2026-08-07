# ADR-0012: Secrets delivered via CSI, never through Muninn's own process

**Status:** Accepted

## Context

The resolver's data path is deliberately secret-free. The cache it serves
is built from ConfigMap-sourced data, and the query API performs no
caller authentication ([ADR-0003](0003-no-caller-auth-on-query-api.md))
on the premise that nothing flowing through it is a credential or grants
access to anything else. That premise is load-bearing: any object type
whose values reach the cache reaches every unauthenticated caller of the
API with it.

Consumers nonetheless need secrets alongside their configuration,
delivered with the same "no client code required" property the existing
file-delivery mechanism gives plain configuration. This record answers
how, without weakening the invariant the query API's access model rests
on.

## Decision

A secret is never represented in a ConfigMap by its value, only by a
*reference*: a key ending in a fixed suffix, whose value is a URI naming
where the real secret lives in an external store. At Pod admission, the
same webhook that already builds the plain-config delivery path also
scans resolved configuration for these reference keys and derives a
per-namespace object describing which secrets a CSI secret-store driver
should fetch: the object names *what* to fetch, never carries a fetched
value. The driver and its store-specific provider plugin perform the
actual mount, entirely outside the resolver's own process: a secret
value's only journey is from the external store, through the kubelet, to
the consumer Pod's filesystem.

```mermaid
flowchart TB
    subgraph muninn["Resolver's trust boundary: a secret value never crosses into this box"]
        CM["ConfigMap: log_level, api_url,<br/>plus secret *references* only"]
        Watcher["ConfigSource watcher"]
        Cache[("In-memory cache")]
        API["gRPC Query/Resolve/Describe API"]
        Webhook["Mutating admission webhook"]
        Derived["Generated per-namespace object:<br/>which secret to fetch, not its value"]
        CM --> Watcher --> Cache --> API
        Cache --> Webhook --> Derived
    end
    subgraph external["CSI driver + secret store: where the actual value flows"]
        Driver["Secrets-store CSI driver"]
        Provider["Store-specific provider plugin"]
        Store[("External secret store")]
        Driver --> Provider --> Store
    end
    Derived -. "read by the driver - a routing instruction, not a secret" .-> Driver
    Driver -->|"the actual secret value"| Pod["Consumer Pod's filesystem"]
    API -->|"reference strings only, never a value"| Caller["Direct gRPC callers"]
```

## Alternatives considered

- **A general Secret-watching configuration source.** Rejected outright:
  any object type feeding real secret material into the resolver's own
  cache reopens the invariant ADR-0003 depends on, regardless of how the
  watching mechanism is implemented or how narrowly RBAC scopes it. RBAC
  governs who may watch the objects; it says nothing about who may call
  the unauthenticated API those values would then be served through.
- **The resolver itself calling out to the external store and caching
  the result**, rather than only generating a reference for the CSI
  driver to act on. Rejected for the same reason as the point above:
  the mechanism differs, but a secret value would still transit and rest
  inside the resolver's own process at some point, which is exactly the
  property this design exists to avoid.
- **A second, purely illustrative pluggable source unrelated to
  secrets**, added only to demonstrate the source interface generalizes
  beyond ConfigMaps. Rejected as a weaker use of the same effort: it
  proves something a reviewer already assumes the interface can do,
  where orchestrating a CSI driver's lifecycle demonstrates a
  significantly less commonly understood piece of the Kubernetes storage
  model.

## Consequences

The resolver's own data path stays provably secret-free: every
invariant ADR-0003 already established continues to hold exactly as
before, even though secret delivery exists as a capability now. The
tradeoff is a real orchestration responsibility the webhook takes on
without taking on the underlying security responsibility: it must derive
a *correct* routing object from a namespace's reference keys, but never
needs to be trusted with, or capable of leaking, an actual secret value.
That responsibility belongs entirely to the CSI driver and the external
store's own access controls.

Two deployment postures follow directly from who owns that derived
object's lifecycle: the webhook can create and keep it in sync
automatically, or a platform team can pre-provision it and have the
webhook only validate against it. Each has a different RBAC footprint,
documented separately from this record.

A real limitation falls out of the mechanism, not a design choice: a
mount performed this way is immutable for the life of the Pod, so a
reference added to a ConfigMap after a Pod is already running can be
observed and surfaced, but not retroactively applied. Picking it up
needs a Pod restart, an operator's call to make, not something automated
into a silent rewrite.

This repository implements exactly one store-specific provider plugin
end to end, with the reference URI's scheme documented as the extension
point for adding another: broad multi-store support was never a goal
here, demonstrating the orchestration pattern itself was.
