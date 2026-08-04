package kube

import (
	"context"
	"fmt"
	"maps"
	"time"

	"go.uber.org/zap"
	health "google.golang.org/grpc/health"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

// Watcher watches ConfigMaps (scoped by a configurable label selector) via
// controller-runtime informers and keeps the in-memory app.Cache in sync.
type Watcher struct {
	crCache  ctrlcache.Cache
	appCache *app.Cache
	metrics  *observability.Metrics
	mainHS   *health.Server
	probeHS  *observability.StandaloneHealth
	log      *zap.Logger

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
// to ConfigMaps matching cfg.ConfigMapLabelSelector.
func NewWatcher(
	restCfg *rest.Config,
	scheme *runtime.Scheme,
	appCache *app.Cache,
	metrics *observability.Metrics,
	mainHS *health.Server,
	probeHS *observability.StandaloneHealth,
	log *zap.Logger,
	cfg *config.Config,
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

	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}

	selector, err := labels.Parse(cfg.ConfigMapLabelSelector)
	if err != nil {
		return nil, fmt.Errorf("parsing configmap label selector %q: %w", cfg.ConfigMapLabelSelector, err)
	}

	c, err := ctrlcache.New(restCfg, ctrlcache.Options{
		Scheme: scheme,
		ByObject: map[client.Object]ctrlcache.ByObject{
			&corev1.ConfigMap{}: {Label: selector},
		},
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
	}, nil
}

// Start registers informer handlers, starts the cache, waits for sync,
// seeds appCache from the initial lis, then marks the service ready.
func (w *Watcher) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(context.Background())

	if err := w.registerInformerHandlers(w.ctx, &corev1.ConfigMap{}, "ConfigMap", w.onConfigMapUpsert, w.onConfigMapDelete); err != nil {
		w.cancel()
		return err
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

// SeedCache performs an explicit List call after sync to ensure no events
// were missed between handler registration and cache sync completion.
func (w *Watcher) SeedCache(ctx context.Context) error {
	var cmList corev1.ConfigMapList
	if err := w.crCache.List(ctx, &cmList); err != nil {
		return err
	}

	for i := range cmList.Items {
		w.onConfigMapUpsert(&cmList.Items[i])
	}

	w.log.Info("seeded cache from initial list",
		zap.Int("configmaps", len(cmList.Items)),
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

func (w *Watcher) onConfigMapUpsert(obj any) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok || cm == nil {
		w.log.Warn("received non-ConfigMap object in upsert handler; ignoring")
		return
	}

	w.applyPatch(configPatch{
		namespace: cm.Namespace,
		source:    cm.Name,
		data:      toAnyMap(cm.Data),
		revision:  cm.ResourceVersion,
		updated:   time.Now().UTC(),
	})

	w.log.Debug("upserted configmap",
		zap.String("namespace", cm.Namespace),
		zap.String("name", cm.Name),
		zap.String("resourceVersion", cm.ResourceVersion),
	)
}

func (w *Watcher) onConfigMapDelete(obj any) {
	cm := extractConfigMap(obj)
	if cm == nil {
		w.log.Warn("received unrecognised ConfigMap delete object; ignoring")
		return
	}

	w.applyPatch(configPatch{
		namespace: cm.Namespace,
		source:    cm.Name,
		clearData: true,
		revision:  cm.ResourceVersion,
		updated:   time.Now().UTC(),
	})

	w.log.Debug("deleted configmap",
		zap.String("namespace", cm.Namespace),
		zap.String("name", cm.Name),
	)
}

func toAnyMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}

	return out
}

// extractConfigMap handles both direct objects and DeletedFinalStateUnknown
// tombstones that the informer may deliver on delete events.
func extractConfigMap(obj any) *corev1.ConfigMap {
	if cm, ok := obj.(*corev1.ConfigMap); ok {
		return cm
	}

	inner := unwrapTombstone(obj)
	if cm, ok := inner.(*corev1.ConfigMap); ok {
		return cm
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
