# Verification

The image and the chart are published to GHCR and signed with
[cosign](https://github.com/sigstore/cosign) in keyless mode. The image
additionally carries an SBOM and build provenance. There is no public key to
distribute and no key material to rotate: the signing certificate is issued
against the publishing workflow's GitHub Actions OIDC token, and the trust
anchor is that workflow identity rather than a key.

Four claims can be checked, and they are not all checked the same way:

| Claim | Artifact | Command |
|---|---|---|
| Who published this image | `ghcr.io/garoze/muninn` | `cosign verify` |
| What the image contains | SBOM attestation | `cosign verify-attestation` |
| What source built it | provenance attestation | `gh attestation verify` |
| Who published this chart | `ghcr.io/garoze/charts/muninn` | `cosign verify` |

Two tools are needed because the two attestations live in different places.
The SBOM is an OCI artifact in the registry, attached to the image digest.
Provenance is held in this repository's attestation store, not in the
registry - the two would otherwise contend for the same location, and the
publish that writes second silently replaces the other.

## The identity to pin

Keyless verification is meaningless without asserting who signed, so `cosign`
refuses to run without both of these:

| Assertion | Value |
|---|---|
| `--certificate-oidc-issuer` | `https://token.actions.githubusercontent.com` |
| `--certificate-identity` | `https://github.com/Garoze/Muninn/.github/workflows/publish.yml@refs/tags/<tag>` |

> [!IMPORTANT]
> The identity carries the repository name as GitHub spells it,
> `Garoze/Muninn`, while every OCI reference below is lowercase
> (`ghcr.io/garoze/muninn`). GHCR requires a lowercase path; the OIDC subject
> is not a path and is case-sensitive. An identity written to match the image
> reference will not match the certificate.

`gh attestation verify` takes the repository instead, and derives the rest.

## Verifying the image

Signatures cover a digest, never a tag. Passing a tag verifies whatever it
resolves to at that moment, so where the result is going to be acted on,
resolve the digest first and verify that:

```bash
digest=$(crane digest ghcr.io/garoze/muninn:v0.2.2)
cosign verify ghcr.io/garoze/muninn@"$digest" \
  --certificate-identity "https://github.com/Garoze/Muninn/.github/workflows/publish.yml@refs/tags/v0.2.2" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

A successful check reports which claims were validated:

```
Verification for ghcr.io/garoze/muninn@sha256:f4c09051... --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - The code-signing certificate was verified using trusted certificate authority certificates
```

The digest is an OCI index covering `linux/amd64` and `linux/arm64/v8`. It is
the index that is signed, so one verification covers both platforms.

`latest` moves only on an official release; a prerelease never receives it.
Verifying it still means naming the release tag that last moved it, so the
pattern form below is the workable one in a script.

## Verifying the SBOM

An SPDX document, published as a signed in-toto attestation and covering the
image's dependency graph:

```bash
cosign verify-attestation --type spdxjson ghcr.io/garoze/muninn@"$digest" \
  --certificate-identity "https://github.com/Garoze/Muninn/.github/workflows/publish.yml@refs/tags/v0.2.2" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" > sbom.json
```

> [!NOTE]
> Redirect the output. `cosign verify-attestation` prints the verification
> summary to stderr and the attestation payload - around a megabyte - to
> stdout, and does not exit if nothing drains that stdout.

The payload is a base64 in-toto statement; the SPDX document is its predicate:

```bash
jq -r .payload sbom.json | base64 -d | jq '.predicate.packages | length'
```

## Verifying the provenance

Provenance names the workflow, the repository and the commit that produced the
image, as a SLSA v1 statement:

```bash
gh attestation verify oci://ghcr.io/garoze/muninn@"$digest" --repo Garoze/Muninn
```

> [!NOTE]
> `gh attestation verify` prints nothing and exits 0 when stdout is not a
> terminal. Assert on `--format json` rather than on the absence of output.

```bash
gh attestation verify oci://ghcr.io/garoze/muninn@"$digest" --repo Garoze/Muninn --format json \
  | jq -r '.[0].verificationResult.statement.predicate.buildDefinition.externalParameters.workflow'
```

This reaches SLSA Build **L2**, not L3. L3 requires the signing material to be
unreachable from user-defined build steps; here the build and the attestation
share a job and its token, so the build could in principle forge its own
provenance. The distinction is about isolation between build and signing, not
about the provenance carrying less information.

## Verifying the chart

The chart's version is independent of the release tag: a release in which
nothing under `charts/` changed republishes the chart at its existing version
while the image advances. The identity that signed a given chart digest
therefore names whichever release tag last published it, which the chart
version does not reveal. Constrain the workflow and the tag shape instead of a
single tag:

```bash
chart_digest=$(crane digest ghcr.io/garoze/charts/muninn:0.2.0)
cosign verify ghcr.io/garoze/charts/muninn@"$chart_digest" \
  --certificate-identity-regexp "^https://github.com/Garoze/Muninn/.github/workflows/publish.yml@refs/tags/v.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

The regular expression is anchored deliberately. Unanchored, it matches any
identity containing that substring, which is a materially weaker assertion
than the one it appears to make.

To learn which release published a chart digest, read the subject off the
verified signature:

```bash
cosign verify ghcr.io/garoze/charts/muninn@"$chart_digest" \
  --certificate-identity-regexp "^https://github.com/Garoze/Muninn/.github/workflows/publish.yml@refs/tags/v.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" 2>/dev/null \
  | jq -r '.[].optional.Subject'
```

## Installing a verified chart

Helm accepts an OCI digest reference, so the digest that was verified can be
the digest that is installed:

```bash
helm install muninn oci://ghcr.io/garoze/charts/muninn@"$chart_digest" \
  --namespace muninn-system --create-namespace
```

Verifying by version tag and then installing by version tag leaves a window in
which the tag can move between the two commands. Installing the digest closes
it.

## What verification establishes, and what it does not

A passing set of checks establishes that this image and chart were built and
published by this repository's workflow, at a named tag, from a named commit,
and have not been altered since; and that the SBOM describes that exact
digest.

It does not establish:

- **That the workflow itself is trustworthy.** The identity names the workflow
  file and the tag it ran at. Both are readable in this repository, and
  reading them is part of the check rather than something the check performs.
- **That the image the chart deploys is the image that was verified.** The two
  signatures are independent, and no attestation binds one to the other. The
  chart's default `image.tag` is `latest`, a reference that moves with each
  official release, so an install that verified a digest and then accepted
  that default is running something other than what was checked. Closing the
  half of this that a consumer controls means pinning the verified digest at
  install time:

  ```bash
  helm install muninn oci://ghcr.io/garoze/charts/muninn@"$chart_digest" \
    --namespace muninn-system --create-namespace \
    --set image.digest="$digest"
  ```

  Set alongside the tag rather than instead of it: the digest is what
  resolves, and the tag stays readable in the rendered reference.

  > [!IMPORTANT]
  > `image.digest` exists from chart `0.2.1`. Helm accepts a value a chart
  > does not define without complaint, so setting it on an earlier chart
  > silently installs the floating tag instead. Confirm the rendered
  > reference with `helm template ... --set image.digest=...` rather than
  > assuming the flag took effect.
- **Anything about the chart's contents.** The chart is signed but carries
  neither an SBOM nor provenance, so which images an install pulls onto a
  cluster - including the opt-in cert-manager and CSI driver subcharts - is
  not covered by any attestation.
- **That an unverified artifact would be refused at install or admission
  time.** Nothing in the chart enforces signature policy on a cluster; that
  belongs to an admission controller the operator chooses and this chart does
  not install.

## Where these checks already run

- The publishing workflow verifies each signature and attestation it produces,
  under the identity it just signed with, before the job succeeds.
- The [nightly workflow](testing.md#nightly) verifies both signatures and both
  attestations against the published artifacts, and installs only after they
  pass. It is the tier that matters most here: an attestation can be replaced
  after a publish has already reported success, so a green publish is not by
  itself evidence that the artifact set survived.
