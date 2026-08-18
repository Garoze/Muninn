# ADR-0001: Patch-based cache merge across independently-owned source objects

**Status:** Accepted

## Context

Any number of source objects (ConfigMaps today, any registered source
kind in general) can exist in the same scope, watched independently and
updated asynchronously. A scope's cached state must reflect the current
union of every source object backing it, and no source's watcher should
need to know the shape of any other source's data.

## Decision

Each incoming event merges only the fields its source object owns into
the scope's existing cache entry, rather than replacing the entry
wholesale.

## Alternatives considered

- **Replace the whole cached entry on every event.** Rejected: a change
  to one source object would discard every other source object's
  contribution to that scope without reporting it, since only one event's
  data is available at the moment of replacement.
- **One shared, directly-mutated state object written by every
  watcher.** Rejected: couples every watcher to the full state shape
  rather than only the portion it owns.

## Consequences

No single event carries a complete picture of a scope. Determining a
scope's current state requires reading the merged result rather than any
individual event. In exchange, each source remains decoupled from every
other source's data shape. Each source's contribution is keyed by a
cache-facing identity distinct from its externally reported type, so
sources sharing an object name in the same scope do not collide,
including two independently registered sources of the same type, which a
type-only key cannot distinguish (see
[ADR-0008](0008-pluggable-config-source.md) for that distinction and for
why a same-type collision fails at startup rather than merging without
report). The rest of the caching model builds on this decision, including
how source-object deletion is handled: a scope's entry is removed once
every source object backing it is gone, with no source treated as a
special-cased identity anchor.

Because each source's watch delivers events on its own goroutine, the
merge must be atomic per scope. Performing the read of a scope's entry,
the merge into a copy, and the store of the result as separate steps
allows two sources to interleave and lose one contribution: the same
data loss this decision prevents, reached by a different route than the
keying collision above. The merge therefore holds the scope's write lock
across the entire read-modify-write rather than around each step. Go's
race detector does not report this class of loss, since each individual
step is already guarded; a test covers it by running concurrent merges
and asserting that every source's contribution survives.

A source that still exists but currently contributes no keys remains
distinct from one that has been removed: the first empties its own slice,
the second removes it. Treating the two identically would leave a scope
serving values that its source object no longer contains.
