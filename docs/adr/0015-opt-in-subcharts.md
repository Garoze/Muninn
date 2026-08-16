# ADR-0015: Cluster-singleton prerequisites ship as opt-in dependencies, default off

**Status:** Accepted

## Context

Two pieces of third-party infrastructure are needed for the full feature
set. The admission webhook cannot serve without a certificate the API
server trusts, and delivering the values behind secret references
([ADR-0012](0012-csi-secret-delivery.md)) needs a secret-store driver and
a provider on every node.

Both are cluster singletons. A certificate manager and a node-level
storage driver are installed once per cluster, usually before this
project exists there, and frequently by someone else. An installation
that assumes their absence and installs them will collide with the copy
already running; one that assumes their presence fails on a cluster that
has neither, with an error pointing at a missing resource type rather
than at the prerequisite that is actually missing.

## Decision

Both ship as declared dependencies of the chart, conditional on a value,
and both default to off. The common case - a cluster where these already
exist, managed independently - installs nothing extra and inherits
nothing. A cluster that has neither can turn them on and get a working
installation from one source.

Off is the default because the failure modes are asymmetric. Installing
alongside an existing singleton damages a shared component other
workloads depend on; declining to install one produces a clear failure in
this project alone.

## Alternatives considered

**Default on.** Optimizes for the empty cluster, which is the rarer case
and the one whose operator is most able to notice what is missing. It
makes the destructive mistake the default, which inverts the asymmetry
above.

**Require them as external prerequisites, documented only.** The honest
minimum, and where this started. It leaves the fresh-cluster path as a
list of manual steps whose ordering matters and which nothing verifies,
and it makes the project's own end-to-end coverage depend on a cluster
someone prepared by hand.

**Install them through a separate mechanism owned by this project.**
Removes the conditional dependency and replaces it with bespoke ordering
logic reimplementing what the packaging tool already does, while still
owning the collision problem.

## Consequences

A fresh install that enables them needs two passes. The certificate
manager's own admission webhook has to be serving before a certificate
request is admitted, and a single pass submits both at once - so the
first pass installs the dependency with this project's webhook disabled,
and the second enables it. This is inherent to admission-time validation,
not to how the dependency is packaged.

Dependencies inherit the release namespace, with no per-dependency
override. Installing them means installing them where this project lives,
which is not where an operator would have put them by choice.

Custom resource definitions supplied by a dependency are installed once
and never upgraded by the packaging tool. Moving a dependency to a
version whose definitions changed requires applying those definitions
outside the normal upgrade, and nothing warns when that has been missed.

Removing this project removes what it installed. On a cluster where the
dependencies were enabled here, uninstalling takes a cluster-wide
certificate manager with it, breaking every unrelated workload that
depended on it.

The dependency archives are vendored into the published package, so
installing pulls images this project does not build and cannot attest.
Which images those are is answerable only by rendering the chart, since
no attestation covers the chart's contents.
