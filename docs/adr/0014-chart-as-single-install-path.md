# ADR-0014: A packaged chart as the single install path

**Status:** Accepted

## Context

Installing Muninn does not produce one fixed set of objects. Whether the
admission webhook is deployed, how its serving certificate is obtained
(cert-manager, self-signed, or supplied by the operator), and whether the
grants secret delivery requires
([ADR-0012](0012-csi-secret-delivery.md)) are present are separate
choices. A static manifest set expresses those choices as a separate
manifest variant per combination, each differing from the others in a
small number of fields.

A static manifest set also cannot express a prerequisite a cluster may
already run. A cluster missing cert-manager or secrets-store-csi-driver
([ADR-0015](0015-opt-in-subcharts.md)) has to install those separately,
in a specific order, before installing Muninn.

## Decision

The Helm chart is the only supported way to install Muninn. The static
manifest set is deleted, the build targets install and uninstall the
chart, and the tests that assert over rendered authorization render it
from the chart rather than reading a checked-in copy.

Demonstration fixtures for a consumer's namespace remain as plain
manifests. They are example input a consumer applies to their own
namespace, not part of installing Muninn.

## Alternatives considered

- **Keep both, treating the manifest set as the readable reference.**
  Rejected: `helm template` renders exactly what an install applies, from
  the source the install uses, so the same readability is available
  without a second set of files to keep current.
- **Keep the manifest set and drop the chart.** Rejected: workable while
  the installation has no options. Each option above would be expressed as
  another manifest variant rather than as a value.
- **Keep both, generating the manifest set from the chart.** Rejected:
  removes the drift, but adds a generation step, a check that the
  generated output is current, and a second artifact to review in every
  change. The generated copy would not be used for installation and would
  therefore introduce an additional artifact to maintain.

## Consequences

Installing now requires Helm, which was not previously required.

The tests that assert over rendered authorization require Helm as well.
They skip rather than fail when it is missing, which loses coverage
without reporting the loss, so CI installs it explicitly rather than
relying on the runner image.

The chart and the image are separate artifacts with independent versions,
and the chart's default image reference is a mutable tag rather than an
immutable digest. A verified chart therefore does not determine which
image will run. The chart accepts a digest for the image reference, and
the verification documentation covers pinning the chart and the image.
