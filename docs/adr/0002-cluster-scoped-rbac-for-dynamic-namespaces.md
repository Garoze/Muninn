# ADR-0002: Cluster-scoped RBAC for a dynamic namespace set

**Status:** Accepted

## Context

The registered `ConfigMap` source is namespace-scoped, but the set of
namespaces containing a matching, labeled ConfigMap is open-ended and
changes at runtime rather than being a fixed list known in advance.

## Decision

Grant access via a single cluster-scoped role bound cluster-wide, rather
than a namespace-scoped role bound per namespace.

## Alternatives considered

- **A namespaced role created alongside each namespace that needs one.**
  Rejected: couples Muninn's access provisioning to whatever creates those
  namespaces, and cannot grant access to a namespace before it exists.

## Consequences

The access grant is broader than any single namespace's resources
require. That is an accepted widening of blast radius in exchange for
expressing "watch this resource type across a namespace set that does not
exist yet." The grant stays narrow along other axes (read-only, scoped
to exactly the resource type the registered source watches, no
subresources), which offsets part of the widening. A bring-your-own
source kind carries its own RBAC requirement as part of registering that
source; this grant covers only the reference `ConfigMap` source.
