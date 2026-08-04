# ADR-0001: Patch-based cache merge across independently-owned source objects

**Status:** Accepted

## Context

Any number of source objects (ConfigMaps today; any registered source
kind in general) can exist in the same scope, watched
independently and updating asynchronously, on their own schedules. A
scope's cached state must reflect the current union of every source
object backing it, and no single source's watcher should need to know
the shape of any other source's data.

## Decision

Each incoming event merges only the fields its source object owns into
the scope's existing cache entry, rather than replacing the entry
wholesale.

## Alternatives considered

- **Replace the whole cached entry on every event.** Rejected: a change
  to one source object would silently discard every other source
  object's contribution to that scope, since only one event's data is
  available at the moment of replacement.
- **One shared, directly-mutated state object written by every
  watcher.** Rejected: couples every watcher to the full state shape,
  instead of only the portion it's responsible for.

## Consequences

No single event ever carries a complete picture of a scope — reasoning
about "what does this scope look like right now" always means reading
the merged result, not any one event. In exchange, every source stays
fully decoupled from every other source's data shape. Each source's
contribution is keyed by that source's kind and object name, so two
different source kinds sharing an object name in the same scope don't
collide. This decision is the foundation the rest of the caching model is
built on, including how source-object deletion is handled — a scope's
entry disappears once every source object backing it is gone, with no
source treated as a special-cased identity anchor.
