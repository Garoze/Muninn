package kube

import (
	"context"
	"fmt"
	"maps"
	"time"

	"go.uber.org/zap"
	health "google.golang.org/grpc/health"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/observability"
)

// Watcher watches a set of pluggable ConfigSources via controller-runtime
// informers and keeps the in-memory app.Cache in sync.
type Watcher struct {
	crCache  ctrlcache.Cache
	appCache *app.Cache
	metrics  *observability.Metrics
	mainHS   *health.Server
	probeHS  *observability.StandaloneHealth
	log      *zap.Logger
	sources  []ConfigSource

	ctx    context.Context
	cancel context.CancelFunc
}

// configPatch carries a source-scoped update for a single namespace.
// Each source (e.g. a ConfigMap) owns its own slice of the cache entry -
// one source's update never clobbers another source's data for the same
// namespace.
type configPatch struct {
	namespace string
	source    string

	data      map[string]any
	clearData bool

	revision string
	updated  time.Time
}

// NewWatcher creates a Watcher backed by a controller-runtime cache, scoped
// to whatever object types and label selectors sources declare.
func NewWatcher(
	restCfg *rest.Config,
	scheme *runtime.Scheme,
	appCache *app.Cache,
	metrics *observability.Metrics,
	mainHS *health.Server,
	probeHS *observability.StandaloneHealth,
	log *zap.Logger,
	sources []ConfigSource,
) (*Watcher, error) {
	if restCfg == nil {
		return nil, fmt.Errorf("nil rest config")
	}

	if scheme == nil {
		return nil, fmt.Errorf("nil scheme")
	}

	if appCache == nil {
		return nil, fmt.Errorf("nil app cache")
	}

	if metrics == nil {
		return nil, fmt.Errorf("nil metrics")
	}

	if log == nil {
		return nil, fmt.Errorf("nil logger")
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no config sources registered")
	}

	byObject := make(map[client.Object]ctrlcache.ByObject, len(sources))
	for _, src := range sources {
		selector, err := labels.Parse(src.LabelSelector())
		if err != nil {
			return nil, fmt.Errorf("parsing label selector for %s source: %w", src.Kind(), err)
		}
		byObject[src.Watch()] = ctrlcache.ByObject{Label: selector}
	}

	c, err := ctrlcache.New(restCfg, ctrlcache.Options{
		Scheme:   scheme,
		ByObject: byObject,
	})
	if err != nil {
		return nil, fmt.Errorf("creating controller-rutime cache: %w", err)
	}

	return &Watcher{
		crCache:  c,
		appCache: appCache,
		metrics:  metrics,
		mainHS:   mainHS,
		probeHS:  probeHS,
		log:      log.Named("watcher"),
		sources:  sources,
	}, nil
}

// Start registers informer handlers, starts the cache, waits for sync,
// seeds appCache from the initial lis, then marks the service ready.
func (w *Watcher) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(context.Background())

	for _, src := range w.sources {
		if err := w.registerInformerHandlers(w.ctx, src.Watch(), src.Kind(), w.onSourceUpsert(src), w.onSourceDelete(src)); err != nil {
			w.cancel()
			return err
		}
	}

	w.metrics.CacheSynced.Set(0)

	errCh := make(chan error, 1)
	go func() {
		if err := w.crCache.Start(w.ctx); err != nil {
			select {
			case errCh <- fmt.Errorf("starting controler-runtime cache: %w", err):
			default:
			}
		}
	}()

	syncedCh := make(chan struct{})
	go func() {
		if w.crCache.WaitForCacheSync(ctx) {
			close(syncedCh)
		}
	}()

	select {
	case <-syncedCh:
	case err := <-errCh:
		w.cancel()
		return err
	case <-ctx.Done():
		w.cancel()
		return fmt.Errorf("context cancelled before cache sync: %w", ctx.Err())
	}

	if err := w.SeedCache(ctx); err != nil {
		w.cancel()
		return fmt.Errorf("seeding cache: %w", err)
	}

	w.appCache.SetSynced()
	w.metrics.CacheSynced.Set(1)
	w.metrics.CacheEntries.Set(float64(w.appCache.Len()))
	observability.MarkHealthServing(w.mainHS, w.probeHS)
	w.log.Info("informers synced and watching",
		zap.Int("namespaces_cached", w.appCache.Len()),
	)
	return nil
}

// Stop cancels the watcher's background context.
func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
		w.metrics.CacheSynced.Set(0)
		w.log.Info("watcher stopped")
	}
}

// SeedCache performs an explicit List call per source after sync, to ensure
// no events were missed between handler registration and cache sync
// completion. apimeta.ExtractList generically pulls the items out of any
// list type following the standard Items-field convention, so this works
// for every source without per-kind code.
func (w *Watcher) SeedCache(ctx context.Context) error {
	total := 0
	for _, src := range w.sources {
		list := src.List()
		if err := w.crCache.List(ctx, list); err != nil {
			return fmt.Errorf("listing %s: %w", src.Kind(), err)
		}

		items, err := apimeta.ExtractList(list)
		if err != nil {
			return fmt.Errorf("extracting %s list items: %w", src.Kind(), err)
		}

		upsert := w.onSourceUpsert(src)
		for _, obj := range items {
			upsert(obj)
		}
		total += len(items)
	}

	w.log.Info("seeded cache from initial list",
		zap.Int("objects", total),
	)

	return nil
}

func (w *Watcher) applyPatch(p configPatch) {
	if p.namespace == "" {
		return
	}

	cur := w.appCache.Get(p.namespace)

	sources := make(map[string]map[string]any)
	revision := p.revision
	if cur != nil {
		maps.Copy(sources, cur.Sources)
		if revision == "" {
			revision = cur.Revision
		}
	}

	if p.data != nil {
		sources[p.source] = p.data
	}

	if p.clearData {
		delete(sources, p.source)
	}

	updated := p.updated
	if updated.IsZero() {
		updated = time.Now().UTC()
	}

	if len(sources) == 0 {
		w.appCache.Delete(p.namespace)
	} else {
		w.appCache.Set(&app.ConfigEntry{
			Namespace: p.namespace,
			Sources:   sources,
			Revision:  revision,
			UpdatedAt: updated,
		})
	}

	w.metrics.CacheEntries.Set(float64(w.appCache.Len()))
}

func (w *Watcher) registerInformerHandlers(
	ctx context.Context,
	obj client.Object,
	resourceName string,
	onUpsert func(any),
	onDelete func(any),
) error {
	informer, err := w.crCache.GetInformer(ctx, obj)
	if err != nil {
		return fmt.Errorf("getting %s informer: %w", resourceName, err)
	}

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			w.metrics.InformerEventsTotal.WithLabelValues("add").Inc()
			onUpsert(obj)
		},
		UpdateFunc: func(_, newObj any) {
			w.metrics.InformerEventsTotal.WithLabelValues("update").Inc()
			onUpsert(newObj)
		},
		DeleteFunc: func(obj any) {
			w.metrics.InformerEventsTotal.WithLabelValues("delete").Inc()
			onDelete(obj)
		},
	}); err != nil {
		return fmt.Errorf("adding %s event handler: %w", resourceName, err)
	}

	return nil
}

// Event Handlers

// onSourceUpsert returns an upsert handler bound to src. Works for any
// controller-runtime object type: every watched object already implements
// client.Object, so no per-kind code is needed here.
func (w *Watcher) onSourceUpsert(src ConfigSource) func(any) {
	return func(obj any) {
		co, ok := obj.(client.Object)
		if !ok || co == nil {
			w.log.Warn("received object without client.Object in upsert handler; ignoring",
				zap.String("kind", src.Kind()),
			)
			return
		}

		w.applyPatch(configPatch{
			namespace: co.GetNamespace(),
			source:    sourceKey(src, co),
			data:      src.Extract(co),
			revision:  co.GetResourceVersion(),
			updated:   time.Now().UTC(),
		})

		w.log.Debug("upserted object",
			zap.String("kind", src.Kind()),
			zap.String("namespace", co.GetNamespace()),
			zap.String("name", co.GetName()),
			zap.String("resourceVersion", co.GetResourceVersion()),
		)
	}
}

func (w *Watcher) onSourceDelete(src ConfigSource) func(any) {
	return func(obj any) {
		co := extractObject(obj)
		if co == nil {
			w.log.Warn("received unrecognised delete object; ignoring",
				zap.String("kind", src.Kind()),
			)
			return
		}

		w.applyPatch(configPatch{
			namespace: co.GetNamespace(),
			source:    sourceKey(src, co),
			clearData: true,
			revision:  co.GetResourceVersion(),
			updated:   time.Now().UTC(),
		})

		w.log.Debug("deleted object",
			zap.String("kind", src.Kind()),
			zap.String("namespace", co.GetNamespace()),
			zap.String("name", co.GetName()),
		)
	}
}

// sourceKey identifies obj's contributed slice within ConfigEntry.Sources.
// Prefixed by the source's Kind so two different object kinds sharing an
// object name in the same namespace don't collide as merge sources.
func sourceKey(src ConfigSource, obj client.Object) string {
	return src.Kind() + "/" + obj.GetName()
}

// extractObject handles both direct objects and DeletedFinalStateUnknown
// tombstones that the informer may deliver on delete events.
func extractObject(obj any) client.Object {
	if co, ok := obj.(client.Object); ok {
		return co
	}

	inner := unwrapTombstone(obj)
	if co, ok := inner.(client.Object); ok {
		return co
	}

	return nil
}

func unwrapTombstone(obj any) any {
	switch t := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		return t.Obj
	case *cache.DeletedFinalStateUnknown:
		return t.Obj
	}

	return nil
}
