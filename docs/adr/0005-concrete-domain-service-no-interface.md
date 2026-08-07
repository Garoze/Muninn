# ADR-0005: Concrete domain service dependency instead of a handler-defined interface

**Status:** Accepted

## Context

Common Go convention favors a consumer defining the interface it
depends on. The transport layer's dependency here is an in-memory,
deterministic lookup with no I/O.

## Decision

The transport layer holds a concrete reference to the domain service,
not an interface it defines for itself.

## Alternatives considered

- **A transport-defined interface satisfied by the concrete service.**
  Rejected: no second implementation exists or is anticipated, and
  mocking a cheap, deterministic dependency would only reimplement its
  logic under test rather than remove a real cost. Interfaces earn
  their keep when the real implementation is expensive or
  non-deterministic (a database, a network call): this isn't.

## Consequences

Slightly against common Go idiom on its face, but avoids indirection
with no corresponding benefit today. If a second concrete
implementation of the domain service appears later: for example, a
caching decorator in front of it: introducing the interface at that
point costs nothing, since interfaces are satisfied implicitly. This is
an explicit, accepted reversal condition rather than a permanent bet.
