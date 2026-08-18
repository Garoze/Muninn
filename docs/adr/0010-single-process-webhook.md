# ADR-0010: Webhook runs as a subcommand of the same binary, deployed as a separate process

**Status:** Accepted

## Context

The mutating admission webhook sits on the Kubernetes API server's
admission path. With `failurePolicy: Fail`, an unreachable webhook blocks
Pod scheduling for every opted-in Pod create in the cluster. The gRPC
resolver has no equivalent criticality: its unavailability degrades one
query at a time rather than cluster-wide scheduling. Precedent exists on
both sides of this question. cert-manager and prometheus-operator ship
their admission webhooks as separate binaries specifically because of
that differing blast radius; kubebuilder's default scaffold and Istio's
`istiod` bundle controllers and webhooks into a single manager process
for operational simplicity.

## Decision

The webhook runs as a mode of the same binary as the gRPC resolver,
selected via a subcommand (`serve` for the resolver, `webhook` for the
admission webhook), rather than as a separate binary released and
versioned independently. The two modes are still deployed as separate
processes (separate Deployments running the same image, with separate
probes and ports), so the differing availability profile above is
preserved at the deployment level even though the source and the release
artifact are shared.

## Alternatives considered

- **A fully separate binary and repository artifact
  (`cmd/muninn-webhook`).** Rejected for this scope: it is the
  conventional answer for a system whose components have genuinely
  divergent availability profiles, but a second binary adds deploy, RBAC,
  and image surface that is not justified while both modes share every
  other dependency (configuration loading, the shared gRPC dial helper,
  structured logging setup) with no requirement to build and release them
  independently at this repository's current scope.

## Consequences

Both modes share configuration loading, logging setup, and the gRPC dial
helper without duplication, and a change to one of those shared pieces
cannot drift between two separately maintained binaries. The cost is that
the two modes' release cadence and binary surface are coupled by
construction: a change scoped to only one mode still rebuilds and
redeploys the same binary artifact as the other. This decision is
revisitable: if the deploy, RBAC, and image surface of a second binary
later becomes worth the operational separation, splitting the two apart
costs a new `cmd/` entrypoint and its chart templates, not a rewrite of
either mode's own logic.
