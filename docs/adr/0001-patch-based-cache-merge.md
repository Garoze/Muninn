# ADR-0001: Patch-based cache merge across independently-owned resources

**Status:** Accepted

## Context

Three custom resource types are watched independently and update
asynchronously, on their own schedules. A tenant's cached state must
reflect the current union of all three, and no single resource type's
watcher should need to know the shape of the other two.

## Decision

Each incoming event merges only the fields its resource type owns into
the tenant's existing cache entry, rather than replacing the entry
wholesale.

## Alternatives considered

- **Replace the whole cached entry on every event.** Rejected: a change
  to one resource type would silently discard the other two resource
  types' contributions, since only one event's data is available at the
  moment of replacement.
- **One shared, directly-mutated state object written by all three
  watchers.** Rejected: couples every watcher to the full state shape,
  instead of only the portion it's responsible for.

## Consequences

No single event ever carries a complete picture of a tenant — reasoning
about "what does this tenant look like right now" always means reading
the merged result, not any one event. In exchange, the three watchers
stay fully decoupled from each other's data shape. This decision is the
foundation the rest of the caching model is built on, including how
resource deletion is handled.
