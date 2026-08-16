# Writing a config source

Muninn watches a set of `ConfigSource`s rather than `ConfigMap` directly. The
watch layer, the cache and the domain layer are written against that interface,
so a bring-your-own custom resource becomes one more registration with no
change to the watcher, the merge, or the gRPC API.

`ConfigMapSource` is the reference implementation and the only source
registered by default. [ADR-0008](adr/0008-pluggable-config-source.md) records
why the interface exists; this document is how to add one.

## The interface

Seven methods, none of which take a Kubernetes client: the watcher owns the
informer, and a source only declares what to watch and how to read it.

| Method | Responsibility |
|---|---|
| `Kind` | Type name, used in logs, `Describe`, and `ENABLED_CONFIG_SOURCES` matching. Sources of the same type share a `Kind` by design. |
| `KeyPrefix` | This source's cache-facing identity, which must be unique across registrations. |
| `Watch` | An empty instance of the object type to watch. |
| `List` | An empty instance of the matching list type, used to seed the cache after sync. |
| `LabelSelector` | Scopes which objects of that type are watched. |
| `Scope` | What the object is scoped by, reported through `Describe`. |
| `Extract` | Pulls configuration data out of one matching object. Return only what a consumer should read as configuration: `ConfigMapSource` reads `data` and ignores `binaryData`, since a configuration value is text. |

## A worked example

Watching a namespaced custom resource whose spec carries a map of values:

```go
type RuntimeConfigSource struct {
	labelSelector string
}

func NewRuntimeConfigSource(cfg *config.Config) *RuntimeConfigSource {
	return &RuntimeConfigSource{labelSelector: cfg.ConfigMapLabelSelector}
}

func (s *RuntimeConfigSource) Kind() string      { return "RuntimeConfig" }
func (s *RuntimeConfigSource) KeyPrefix() string { return "RuntimeConfig" }
func (s *RuntimeConfigSource) Scope() string     { return "namespace" }

func (s *RuntimeConfigSource) Watch() client.Object     { return &examplev1.RuntimeConfig{} }
func (s *RuntimeConfigSource) List() client.ObjectList  { return &examplev1.RuntimeConfigList{} }
func (s *RuntimeConfigSource) LabelSelector() string    { return s.labelSelector }

func (s *RuntimeConfigSource) Extract(obj client.Object) map[string]any {
	rc, ok := obj.(*examplev1.RuntimeConfig)
	if !ok || rc == nil {
		return nil
	}

	out := make(map[string]any, len(rc.Spec.Values))
	for k, v := range rc.Spec.Values {
		out[k] = v
	}
	return out
}
```

### `Extract` distinguishes three outcomes

The return value is a three-state contract, and getting it wrong is the easiest
way to lose data in the merge:

- **`nil`** means this source cannot read the object at all. The merge leaves
  the source's existing slice untouched rather than emptying it, so a type
  assertion that fails does not wipe previously-good data.
- **A non-nil empty map** means the object carries no keys. The merge clears
  the slice. The API server stores an object whose every key was removed as a
  nil map, so returning `nil` for that case would cache the removed keys
  indefinitely.
- **A populated map** replaces this source's slice for that namespace, and
  only that slice.

## Registering it

Add one `fx.Provide` to `internal/kube/module.go`'s value group. Nothing
downstream changes:

```go
fx.Provide(
	fx.Annotate(NewRuntimeConfigSource,
		fx.As(new(ConfigSource)),
		fx.ResultTags(`group:"config_sources"`),
	),
),
```

Both annotations are load-bearing. Without `fx.As`, the constructor is provided
under its concrete type and nothing in the group satisfies the `[]ConfigSource`
consumers ask for; without the group result tag, it never joins the group at
all. Neither failure is visible to a unit test that constructs `Watcher`
directly, because such a test bypasses Fx entirely: run the binary against a
real cluster to confirm the wiring, not just the tests.

A custom type also has to be registered into the scheme in
`internal/kube/scheme.go`, the same way core types and `SecretProviderClass`
already are. Without it the cache cannot build an informer for the type, and
startup fails.

## What to check before it works

**RBAC.** The resolver's ClusterRole grants `configmaps` and nothing else. A
new source needs `get`, `list` and `watch` on its own resource added to it. If
the webhook should see the source too, its own ClusterRole needs the same
grant: the two run under separate ServiceAccounts deliberately, so widening
one does not widen the other. Both are rendered by the chart.

**A distinct `KeyPrefix`.** Each source owns a slice of a namespace's cache
entry, keyed by `KeyPrefix` and object name. Two sources sharing a prefix would
silently overwrite each other, so `NewWatcher` rejects the registration at
construction instead:

```
config sources "ConfigMap" and "RuntimeConfig" both use KeyPrefix "shared":
would silently overwrite each other's cache entries
```

`KeyPrefix` defaults to `Kind` when a source leaves it unset, which is correct
only while one source of that kind is registered. Registering two of the same
kind, as `WithKeyPrefix` and `WithLabelSelector` allow for `ConfigMapSource`,
requires giving each an explicit prefix.

**Namespaced objects only.** The watcher takes the namespace from the object's
own metadata, and a patch carrying an empty namespace is dropped. A
cluster-scoped source therefore contributes nothing today; supporting one means
deciding what scope its data belongs to, which is a design question rather than
an interface gap. See [ADR-0007](adr/0007-namespace-as-resolution-scope.md).

**A conventional list type.** The initial seed extracts items generically from
whatever `List` returns, so the list type needs the standard `Items` field that
generated Kubernetes list types already have.

## How it surfaces at runtime

`Describe` reports every enabled source's `Kind`, `LabelSelector` and `Scope`,
so a new source appears there with no change to the API. `ENABLED_CONFIG_SOURCES`
matches on `Kind`, and can only narrow the registered set, never add to it:
naming a kind that is not registered leaves nothing enabled and fails startup
rather than running with nothing watched.

Each source gets its own informer cache scoped to its own label selector. One
shared cache would key informers by type, so two sources watching the same type
would silently share one selector.

## Testing one

Unit-test `Extract` directly against a fixture object, including the
unreadable-object case, which is the branch that protects the merge.

The interface's composability is covered by a fake source in
`internal/kube`'s own tests, which is the pattern to copy for a new one: it
proves the watcher needs no per-kind code without requiring a real CRD. Real
informer behavior belongs in the integration tier against `envtest`, where
`TestTwoSourcesOfSameKind` already covers co-registered sources.
[`testing.md`](testing.md) describes what each tier is responsible for.
