# ADR-0016: Prereleases from `develop`, official releases from `main`

**Status:** Accepted

## Context

The release path builds, publishes, signs, and attests. It runs against
registry and signing infrastructure that no test tier covers, so the only
thing that exercises it is running it. Official releases alone would run
it a few times a year and leave its defects to surface during a release.

Two externally visible surfaces constrain the answer: the branch a
visitor sees by default, and the image an unpinned install receives
through the `latest` tag. Both should reflect released work rather than
work in progress.

## Decision

Two long-lived branches, each with a single responsibility.

Every change enters through `develop`. Merging there updates a
release-please pull request holding the next prerelease version and its
changelog entry. Merging that pull request tags a prerelease, which
builds and publishes an image and a chart, signs both, and attests the
image, so the whole release path runs on every batch of merged work.

`main` holds official releases only. Cutting one is a manual dispatch
with an explicit `vX.Y.Z`. It consolidates the accumulated prerelease
changelog entries into one entry, tags, publishes, and merges `main` back
into `develop`. The `latest` tag is reserved for official releases,
matched by an exact three-part version rather than by excluding the
prerelease version shapes known at the time the rule was written.

The merge back is automated because it is the direction most often
omitted. A `develop`-to-`main` merge advances only `main`, so anything
committed on `main` sits outside the history of the branch every change
enters through. It runs on any push to `main`, not only at the end of a
cut, because commits reach `main` by other routes as well.

## Alternatives considered

- **A single branch, releases by tag.** Rejected for this scope: it is
  what most comparable CNCF projects run, and it removes every failure
  mode listed below by construction. Prereleases can also be published
  continuously from a trunk, so what this decision provides over that
  option is limited to one property: the default branch and the `latest`
  tag never present work in progress.
- **Two branches, merged back by hand.** Rejected: the original
  arrangement, in which divergence is not reported. Nothing indicates
  that the branches have diverged until something reading `develop`
  cannot find a version that exists on `main`.

## Consequences

The release path runs continuously. Every prerelease is a signed,
attested artifact built by the same pipeline as an official release, and
that repetition is what surfaces the pipeline's defects before a release
depends on them.

Keeping two branches consistent requires machinery that a single branch
would not:

- Commits on `main` are unreachable from `develop` without an explicit
  merge back, which had to be automated after being omitted.
- That merge requires `--no-ff`. A `develop`-to-`main` merge leaves `main`
  strictly ahead, so the merge back fast-forwards and commits nothing.
- The changelog diverges by design, since `develop` keeps one entry per
  prerelease and `main` consolidates them. The conflict occurs on every
  merge back and requires its own resolution rule.
- release-please's version manifest is authoritative on `develop` and
  inherited stale on `main`. It is written deliberately at each cut and
  reset on `develop` afterwards; otherwise the next prerelease computes a
  version below the one just released.
- Tools that default to the repository's default branch select the wrong
  one. Dependabot raised its first pull request against `main`, where
  merging would have reached a release without any prerelease having
  exercised the workflows it changes, and would have left the bump
  invisible to `develop` until the next merge back. It is now pointed at
  `develop` explicitly.

Two prerelease version shapes exist and both have to be handled wherever
a version is inspected: the first prerelease after a release carries no
increment, later ones do. Handling only the incremented shape produced
two defects, one where the `latest` tag was withheld and one where the
changelog was consolidated incorrectly. Both now test for an official
release positively rather than excluding prerelease shapes by name.

A workflow runs only from the branch it is on, so any change to release
tooling has to be considered on both branches, including which branch it
must reach to take effect.
