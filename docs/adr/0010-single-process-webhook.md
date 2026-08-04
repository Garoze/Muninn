# ADR-0010: Webhook runs as a subcommand of the same binary, deployed as a separate process

**Status:** Accepted

## Context

The mutating admission webhook sits on the Kubernetes API server's
admission path: with `failurePolicy: Fail`, if it's unreachable, Pod
scheduling blocks for every opted-in Pod create in the cluster. The gRPC
resolver has no equivalent criticality — its own unavailability degrades
one query at a time, not cluster-wide scheduling. Real precedent exists
on both sides of this question: cert-manager and prometheus-operator
ship their admission webhooks as separate binaries specifically because
of that differing blast radius; kubebuilder's default scaffold, and
Istio's `istiod`, bundle controllers and webhooks into a single manager
process for operational simplicity.

## Decision

The webhook runs as a mode of the same binary as the gRPC resolver,
selected via a subcommand (`serve` for the resolver, `webhook` for the
admission webhook), rather than as an entirely separate binary released
and versioned independently. The two modes are still deployed as
separate processes — separate Deployments, separate images built from the
same source, separate probes and ports — so the differing availability
profile above is preserved at the deployment level even though the
source and release artifact are shared.

## Alternatives considered

- **A fully separate binary and repository artifact
  (`cmd/muninn-webhook`).** Rejected for this scope: the more
  textbook-correct answer for a system where the two components'
  availability profiles genuinely diverge, but a second binary adds
  deploy, RBAC, and image surface not justified while both modes still
  share every other dependency (configuration loading, the shared gRPC
  dial helper, structured logging setup) with nothing to gain from
  building and releasing them independently at this project's current
  scope.

## Consequences

Both modes share configuration loading, logging setup, and the shared
gRPC dial helper without duplicating any of it, and a change to one of
those shared pieces can't drift between two separately maintained
binaries. The cost is that the two modes' release cadence and binary
surface are coupled by construction — a change scoped to only one mode
still rebuilds and redeploys the same binary artifact as the other. This
decision is explicitly revisitable: if the deploy, RBAC, and image
surface of a second binary becomes worth the operational separation
later, splitting the two apart costs a new `cmd/` entrypoint and manifest
set, not a rewrite of either mode's own logic.
