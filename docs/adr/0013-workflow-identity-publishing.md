# ADR-0013: Artifacts published and signed under a workflow identity, not a stored credential

**Status:** Accepted

## Context

Installing Muninn means pulling an image and a chart from a registry. A
successful pull proves the artifact exists under that name. It does not
prove the artifact came from this repository's build.

Closing that gap by conventional means requires two long-lived secrets in
repository settings: a credential to publish with, and a signing key to
sign with. Neither reports its own compromise. An image published with a
stolen registry credential is indistinguishable from any other image, and
a signature made with a stolen key verifies successfully.

## Decision

Images and charts are published to GHCR, authenticated by the token
GitHub Actions issues to the publishing workflow for its own run. Signing
is keyless: cosign requests a short-lived certificate bound to the
workflow's OIDC identity and records the signature in Sigstore's
transparency log. No credential used to publish or sign is stored in
repository settings.

Consumers verify against an identity rather than a key: the publishing
workflow's path in this repository, the ref it ran for, and GitHub's OIDC
issuer. The same identity signs everything published: the image, the
chart, and the image's provenance and SBOM attestations.

## Alternatives considered

- **Docker Hub with a stored access token.** Rejected: it is the registry
  most consumers expect, but it requires a long-lived credential in
  repository settings, and its anonymous pull limits are low enough to
  affect a nightly job that installs the published chart repeatedly. GHCR
  is free for public repositories and already hosts the source.
- **Key-based signing.** Rejected: it works with any CI provider and
  requires no OIDC, but the key has to be stored, rotated, and revoked if
  it leaks, and revocation has to reach consumers who already verified
  with it.
- **Publishing without signing.** Rejected: signing is a single step at
  publication, and the provenance and SBOM attestations attach to it.
  Adding it later requires republishing.

## Consequences

The identity is part of the published contract. Moving or renaming the
publishing workflow changes the identity of everything signed afterwards,
and consumers pinning the previous identity stop verifying successfully,
so a rename is a breaking change even though the artifact is unchanged.

`cosign verify` without `--certificate-identity` and
`--certificate-oidc-issuer` accepts any valid Sigstore signature from any
signer. The verification documentation therefore gives both flags with
values filled in rather than leaving a consumer to derive them.

Signatures and attestations live in the registry against the digest they
describe. Deleting a published version deletes the material that proves
its origin. GitHub hosts attestations at no cost for public repositories
only, so making this repository private would remove them.

Verification proves an artifact came from this workflow. It does not
prove the workflow is trustworthy: anyone able to commit to this
repository can change it. Every action the workflow calls is therefore
pinned to a commit SHA rather than to a tag.
