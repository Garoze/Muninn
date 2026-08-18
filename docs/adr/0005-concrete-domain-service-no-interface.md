# ADR-0005: Concrete domain service dependency instead of a handler-defined interface

**Status:** Accepted

## Context

Common Go convention favors a consuming package defining the interface it
depends on. The transport layer's dependency here is an in-memory,
deterministic lookup with no I/O.

## Decision

The transport layer holds a concrete reference to the domain service,
rather than an interface it defines for itself.

## Alternatives considered

- **A transport-defined interface satisfied by the concrete service.**
  Rejected: no second implementation exists or is anticipated, and
  substituting a mock for a deterministic in-memory dependency would
  reimplement its logic under test rather than remove a real cost.
  An interface is worth the indirection when the real implementation is
  expensive or non-deterministic, such as a database or a network call;
  this dependency is neither.

## Consequences

The arrangement departs from common Go idiom, and avoids indirection with
no corresponding benefit today. If a second concrete implementation of
the domain service appears later (a caching decorator in front of it,
for example), introducing the interface at that point costs nothing,
since Go interfaces are satisfied implicitly. This is an explicit,
accepted reversal condition rather than a permanent commitment.
