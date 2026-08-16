# ADR-0016: A staging branch with published prereleases, and an official release branch

**Status:** Accepted

## Context

The release path is the part of a project least exercised and most
expensive to get wrong: it runs rarely, it runs on infrastructure the
tests do not cover, and its defects are discovered at exactly the moment
they are least welcome. Deciding where releases come from is therefore
also deciding how often the machinery that produces them runs.

A second, quieter requirement pulls in the same direction. Anyone
arriving at this repository sees one branch by default, and the artifact
a floating tag resolves to is what an unpinned install gets. Neither
should ever show work in progress, whatever is being built at the time.

## Decision

Two long-lived branches, each with one job.

Every change enters through the staging branch. Merging there updates a
release pull request holding the next prerelease version and its
changelog entry; merging *that* tags a prerelease, which publishes an
image and a chart, signs both, and attests the image — the entire release
path, on every batch of merged work.

The release branch holds only official releases. Cutting one is a
deliberate act: it consolidates the accumulated prerelease changelog
entries into a single entry, tags, publishes, and returns the release
branch's own commits to the staging branch. The floating tag is reserved
for these, matched by an exact three-part version rather than by
excluding the prerelease shapes anyone thought to enumerate.

The direction that is easy to forget is automated. A staging-to-release
merge advances only the release branch; the staging branch's history
never moves, so anything committed on the release side is invisible to
the branch every change enters through. A merge in the other direction
runs on any push to the release branch, not only at the end of a release,
because commits arrive there by more routes than a release cut.

## Alternatives considered

**A single branch, releases by tag.** What most comparable projects run,
and it removes every failure listed below by construction: there is no
second branch to fall behind, no direction to merge back, no second copy
of a changelog to reconcile, and no default-branch ambiguity for tools
that pick one. Continuous exercise of the release path is achievable
without a staging branch, by publishing prereleases from the trunk. The
property this decision is actually buying is narrower than it first
appears: that the default branch and the floating tag never show
in-progress work.

**Two branches, merged back by hand.** The original shape, and the one
that fails silently. Nothing signals that the branches have diverged
until something that reads the staging branch's history cannot find a
version that exists on the other one.

## Consequences

The release path runs continuously, which is the point. Every prerelease
is a signed, attested artifact produced by the same pipeline as an
official release, and that repetition is what surfaces its defects before
a release depends on them.

The machinery that keeps two branches consistent exists only because
there are two branches, and it is not small. Its cost is recorded here
rather than in a commit message, because it is the honest measure of this
decision:

- Commits made on the release branch are unreachable from staging without
  an explicit merge back, which had to be automated after being missed.
- The automation that does the merging had its own defect: a
  staging-to-release merge leaves the release branch strictly ahead, so
  the merge back fast-forwards and produces nothing to commit.
- The changelog diverges by design — staging accumulates one entry per
  prerelease, the release branch consolidates them — so the conflict is
  guaranteed on every merge back and needs its own resolution rule.
- The release automation's version manifest is authoritative on staging
  and inherited stale on the release branch, which had to be corrected
  and then written deliberately at each cut.
- Tooling that defaults to the repository's default branch picks the
  wrong one. A dependency updater raised its first pull request against
  the release branch, where a merge would have reached a release without
  the pipeline it changes having been exercised.

Every one of those is a consequence of the topology rather than a bug in
any single component. A single-branch model would not have produced any
of them.

Two prerelease version shapes exist, and both must be handled everywhere
a version is inspected: the first prerelease after a release carries no
increment, and later ones do. Assuming the incremented shape has been
wrong twice — once where the floating tag was withheld, once where the
changelog was consolidated — which is why both now test for an official
release positively rather than excluding prereleases by name.

Anything that changes release tooling has to be considered on both
branches, including which branch it must reach to take effect. A workflow
runs only from the branch it exists on, so a fix to the release branch's
behaviour does nothing until it is merged there.
