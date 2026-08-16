# ADR-0013: Artifacts published and signed under a workflow identity, not a stored credential

**Status:** Accepted

## Context

Nothing was published anywhere: the image existed only in one machine's
local container store, and the manifests referred to it by a name no
other cluster could resolve. Making the project installable means
choosing a registry, and choosing a registry immediately raises a second
question that looks separate and is not - how a consumer knows that what
they pulled is what this project built.

The two questions share an answer because they share a credential. A
registry that requires a stored username and password puts a long-lived
secret in the repository's settings, and signing with a key pair puts a
second one there. Both are credentials whose compromise is silent: an
artifact published with a stolen registry password is indistinguishable
from a legitimate one, and an artifact signed with a stolen key verifies
correctly.

## Decision

Artifacts are published to the registry operated by the same forge that
hosts the source, authenticated by the token the publishing workflow is
issued for its own run, and signed keyless against the workflow's OIDC
identity. No registry credential and no signing key is stored anywhere.

What a consumer pins is therefore not a key but an identity: the
publishing workflow, in this repository, at the tag it ran for, as
asserted by the forge's OIDC issuer and recorded in a public transparency
log. A signature proves that this artifact was produced by that workflow
at that ref, and that nothing has altered it since.

The same identity signs everything published - the image, the chart, and
the attestations describing the image - so a consumer learns one identity
and checks every artifact against it.

## Alternatives considered

**A general-purpose registry with a stored credential.** The conventional
choice, and the one most consumers expect. It requires a long-lived
credential in repository settings, its unauthenticated pull limits are
low enough to matter for a scheduled test that installs the published
artifact repeatedly, and it separates the artifact's location from the
source's, so nothing but convention connects them.

**Key-based signing.** A key pair is understood by every tool and works
without a forge. It also has to live somewhere, be rotated, and be
revoked if leaked, and its revocation has to reach consumers who already
verified with it. Keyless signing removes the object that has to be
protected rather than protecting it better.

**Publishing without signing.** Defensible for a project nobody installs,
and it makes every later claim about provenance unavailable. Signing at
publication costs one step, and it is the precondition for attesting
anything about the artifact afterwards.

## Consequences

The identity is part of the published contract. Renaming or moving the
publishing workflow changes the identity of everything signed afterwards,
and consumers pinning the old one will fail to verify - a rename is a
breaking change to verification even though nothing about the artifact
changed.

Verification requires naming both the identity and the issuer. Omitting
them makes the check assert only that *some* valid signature exists,
which is close to no assertion at all, so the tooling refuses to run
without them.

Signatures and attestations are themselves artifacts in the registry,
stored against the digest they describe. Deleting a published version
deletes what proves it, and any retention policy applied to old
prereleases has to account for that.

Attestations are hosted by the forge for public repositories on its free
plans. Making the repository private removes them, which is a licensing
and visibility decision with a supply-chain consequence attached.

The trust anchor is the workflow definition, which is readable in the
repository and mutable by anyone who can commit to it. Verification
establishes that this workflow produced the artifact; it does not
establish that the workflow deserves the trust, which is why the actions
it invokes are pinned to immutable references rather than to tags.
