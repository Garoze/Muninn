# ADR-0014: A packaged chart as the single install path

**Status:** Accepted

## Context

Installation was described twice. A directory of raw manifests carried
the deployment, its service account and role bindings, the webhook's own
deployment and admission registration, and its certificate; a packaged
chart, added later, rendered the same objects from values. Both were
current, both were referenced by the build targets, and neither was
authoritative.

Two descriptions of one installation drift, and they had. Sixteen
manifests encoded ordering and cross-references that the chart also
encoded, and each fix had to be made twice or made once and forgotten.
That is not a hypothetical: the raw manifests and the chart had already
disagreed about which grants the webhook needed, and the tests exercised
whichever copy they happened to reference rather than the one an operator
would apply.

## Decision

The chart is the only way to install. The raw manifest set is deleted,
every build target and every test tier that installed something installs
the chart, and the tests that assert over rendered authorization read
that rendering from the chart rather than from a checked-in copy.

Demonstration fixtures for a *consumer's* namespace stay as plain
manifests. They are not part of installing this project — they are
example input a user applies to their own namespace — and a chart that
installed them would be installing someone else's objects.

## Alternatives considered

**Keep both, and treat the manifests as the readable reference.** The
appeal is that a manifest can be read directly while a template cannot.
The cost is the drift that motivated this record, and the readability is
recoverable anyway: rendering the chart locally produces exactly the
manifests an install applies, from the single source that is actually
deployed.

**Keep the manifests and drop the chart.** Viable while the installation
had no options. It stops being viable once the certificate strategy, the
webhook's presence, and the secret-delivery grants are all choices, since
each one becomes a set of near-identical files a reader has to diff to
understand.

**Keep both, with one generated from the other.** Removes the drift and
adds a generation step, a check that the generated copy is current, and a
second artifact to review in every change. The generated copy would exist
only to be read, which rendering already provides on demand.

## Consequences

Installing requires a chart tool. That is an added dependency for an
operator who previously needed only the cluster CLI, and it is also the
tool that makes the options above expressible at all.

Test tiers that render authorization now depend on that tool too. Where
it is absent those tests skip rather than fail, which silently loses
coverage, so it is installed explicitly in the environment that runs
them.

The published chart and the published image are separate artifacts with
separate versions. Nothing in the chart binds it to a particular image
digest, so a verified chart does not tell a consumer which image will
run — a gap recorded in the verification documentation rather than hidden
by it.

Deleting the manifests removed comments that explained real constraints.
Those explaining something still true were carried into the chart
verbatim; two describing the old duplication itself were dropped, since
the duplication they warned about no longer exists.
