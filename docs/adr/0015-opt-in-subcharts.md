# ADR-0015: Cluster-singleton prerequisites ship as opt-in dependencies, default off

**Status:** Accepted

## Context

Two pieces of third-party infrastructure are required for the full
feature set. The admission webhook cannot serve without a certificate the
API server trusts, which cert-manager supplies. Delivering the values
behind secret references ([ADR-0012](0012-csi-secret-delivery.md))
requires secrets-store-csi-driver and a provider on every node.

Both are cluster singletons. They are installed once per cluster, usually
before Muninn exists there and often by another team. An install that
assumes they are absent collides with the instance already running. An
install that assumes they are present fails on a cluster that has
neither, and the resulting error names a missing resource type rather
than the missing prerequisite.

## Decision

Both ship as chart dependencies, conditional on a value and defaulting to
off. A cluster that already has them installs nothing extra. A cluster
that has neither can enable them and complete the install in one command.

Off is the default because the two failure modes differ in blast radius.
Installing over an existing singleton affects a component other workloads
depend on. Declining to install one affects only Muninn.

## Alternatives considered

- **Default on.** Rejected: optimizes for the empty cluster, which is the
  rarer case, and makes the higher-blast-radius outcome the default.
- **Require them as external prerequisites, documented only.** Rejected:
  it is the smallest possible commitment and the original arrangement,
  but it leaves a fresh cluster with manual, order-dependent steps that
  nothing verifies, and it makes Muninn's end-to-end coverage depend on a
  cluster prepared by hand.
- **Install them from Muninn's own code, outside the chart.** Rejected:
  replaces a conditional dependency with ordering logic that reimplements
  what Helm already provides, and retains the collision problem.

## Consequences

Enabling cert-manager on a fresh cluster takes two passes. Its webhook
has to be serving before a `Certificate` is admitted, and a single pass
submits both at once. The first pass installs the dependency with
Muninn's webhook disabled; the second enables it.

Dependencies are installed into the release namespace, with no
per-dependency override. Enabling them places a cluster-wide component
where Muninn lives rather than where an operator would otherwise have
placed it.

Helm installs a dependency's CRDs once and does not upgrade them. Moving
to a dependency version whose CRDs changed means applying them outside
the normal upgrade, and nothing reports when that step is missed.

Uninstalling Muninn removes what it installed. On a cluster where these
dependencies were enabled here, the uninstall also removes cert-manager
and breaks unrelated workloads.

The dependency archives are packaged into the published chart, so
installing pulls images Muninn does not build and cannot attest.
Rendering the chart is the only way to enumerate them.
