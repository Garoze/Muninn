package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	health "google.golang.org/grpc/health"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/garoze/muninn/api/v1alpha1"
	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/observability"
)

// Watcher watches Tenant, TenantConfig, and Policy CRDs via controller-runtime
// Informers and keeps the in-memory app.Cache in sync.
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

// tenantPatch carries a resource-scoped update for a single tenant.
// Each CRD handler sets only the fields it orns - a Policy update
// never touches RuntimeConfig, and vice versa.
type tenantPatch struct {
	tenantID string

	displayName    *string
	runtimeConfig  map[string]any
	cloudResources map[string]any
	authzPolicy    *app.AuthzPolicySnapshot

	clearDisplayName    bool
	clearRuntimeConfig  bool
	clearCloudResources bool
	clearAuthzPolicy    bool

	revision string
	updated  time.Time
}

// NewWatcher creates a Watcher backed by a controller-runtime cache.
func NewWatcher(
	cfg *rest.Config,
	scheme *runtime.Scheme,
	appCache *app.Cache,
	metrics *observability.Metrics,
	mainHS *health.Server,
	probeHS *observability.StandaloneHealth,
	log *zap.Logger,
) (*Watcher, error) {
	if cfg == nil {
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

	c, err := ctrlcache.New(cfg, ctrlcache.Options{Scheme: scheme})
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

	if err := w.registerInformerHandlers(w.ctx, &v1alpha1.Tenant{}, "Tenant", w.onTenantUpsert, w.onTenantDelete); err != nil {
		w.cancel()
		return err
	}

	if err := w.registerInformerHandlers(w.ctx, &v1alpha1.TenantConfig{}, "TenantConfig", w.onTenantConfigUpsert, w.onTenantConfigDelete); err != nil {
		w.cancel()
		return err
	}

	if err := w.registerInformerHandlers(w.ctx, &v1alpha1.Policy{}, "Policy", w.onPolicyUpsert, w.onPolicyDelete); err != nil {
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
		zap.Int("tenants_cached", w.appCache.Len()),
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

// SeedCache performs explicit List calls after sync to ensure no events
// were missed between handler registration and cache sync completion.
func (w *Watcher) SeedCache(ctx context.Context) error {
	var tenantList v1alpha1.TenantList
	if err := w.crCache.List(ctx, &tenantList); err != nil {
		return err
	}

	for i := range tenantList.Items {
		w.onTenantUpsert(&tenantList.Items[i])
	}

	var tcList v1alpha1.TenantConfigList
	if err := w.crCache.List(ctx, &tcList); err != nil {
		return err
	}

	for i := range tcList.Items {
		w.onTenantConfigUpsert(&tcList.Items[i])
	}

	var policyList v1alpha1.PolicyList
	if err := w.crCache.List(ctx, &policyList); err != nil {
		return err
	}

	for i := range policyList.Items {
		w.onPolicyUpsert(&policyList.Items[i])
	}

	w.log.Info("seeded cache from initial list",
		zap.Int("tenants", len(tenantList.Items)),
		zap.Int("tenants_config", len(tcList.Items)),
		zap.Int("policies", len(policyList.Items)),
	)

	return nil
}

func (w *Watcher) applyPatch(p tenantPatch) {
	if p.tenantID == "" {
		return
	}

	cur := w.appCache.Get(p.tenantID)
	if cur == nil {
		cur = &app.TenantState{TenantID: p.tenantID}
	}

	next := *cur

	if p.displayName != nil {
		next.DisplayName = *p.displayName
	}

	if p.runtimeConfig != nil {
		next.RuntimeConfig = p.runtimeConfig
	}

	if p.cloudResources != nil {
		next.CloudResources = p.cloudResources
	}

	if p.authzPolicy != nil {
		next.AuthzPolicy = p.authzPolicy
	}

	if p.clearDisplayName {
		next.DisplayName = ""
	}

	if p.clearRuntimeConfig {
		next.RuntimeConfig = nil
	}

	if p.clearCloudResources {
		next.CloudResources = nil
	}

	if p.clearAuthzPolicy {
		next.AuthzPolicy = nil
	}

	if p.revision != "" {
		next.Revision = p.revision
	}

	if p.updated.IsZero() {
		next.UpdatedAt = time.Now().UTC()
	} else {
		next.UpdatedAt = p.updated
	}

	if next.DisplayName == "" && next.RuntimeConfig == nil && next.CloudResources == nil && next.AuthzPolicy == nil {
		w.appCache.Delete(next.TenantID)
	} else {
		w.appCache.Set(&next)
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

func (w *Watcher) onTenantUpsert(obj any) {
	t, ok := obj.(*v1alpha1.Tenant)
	if !ok || t == nil {
		w.log.Warn("received non-Tenant object in upsert handler; ignoring")
		return
	}

	key := tenantCacheKey(t)
	if key == "" {
		w.log.Debug("tenant missing tenantID and status.namespace; skipping",
			zap.String("resourceVersion", t.ResourceVersion),
		)
	}

	displayName := t.Spec.DisplayName
	w.applyPatch(tenantPatch{
		tenantID:       key,
		displayName:    &displayName,
		cloudResources: toCloudResourcesMap(t.Status.CloudResources),
		revision:       t.ResourceVersion,
		updated:        time.Now().UTC(),
	})

	w.log.Debug("upserted tenant",
		zap.String("tenant_id", key),
		zap.String("resourceVersion", t.ResourceVersion),
	)
}

func (w *Watcher) onTenantDelete(obj any) {
	t := extractTenant(obj)
	if t == nil {
		w.log.Warn("received unrecognied Tenant delete object; ignoring")
		return
	}

	key := tenantCacheKey(t)
	if key == "" {
		return
	}

	w.applyPatch(tenantPatch{
		tenantID:           key,
		clearDisplayName:   true,
		clearRuntimeConfig: true,
		revision:           t.ResourceVersion,
		updated:            time.Now().UTC(),
	})

	w.log.Debug("deleted tenant",
		zap.String("tenant_id", key),
	)
}

func (w *Watcher) onTenantConfigUpsert(obj any) {
	tc, ok := obj.(*v1alpha1.TenantConfig)
	if !ok || tc == nil {
		w.log.Warn("received non-TenantConfig object in upsert handler; ignoring")
		return
	}

	tenantID := tenantIDFromNamespace(tc.Namespace)
	if tenantID == "" {
		w.log.Warn("tenant config missing namespace; ignoring",
			zap.String("resourceVersion", tc.ResourceVersion),
		)
	}

	w.applyPatch(tenantPatch{
		tenantID:      tenantID,
		runtimeConfig: toAnyMap(tc.Spec.RuntimeConfig),
		revision:      tc.ResourceVersion,
		updated:       time.Now().UTC(),
	})

	w.log.Debug("upserted tenant config",
		zap.String("tenant_id", tenantID),
	)
}

func (w *Watcher) onTenantConfigDelete(obj any) {
	tc := extractTenantConfig(obj)
	if tc == nil {
		w.log.Warn("received unrecognised TenantConfig delete object; ignoring")
		return
	}

	tenantID := tenantIDFromNamespace(tc.Namespace)
	if tenantID == "" {
		return
	}

	w.applyPatch(tenantPatch{
		tenantID:           tenantID,
		clearRuntimeConfig: true,
		revision:           tc.ResourceVersion,
		updated:            time.Now().UTC(),
	})

	w.log.Debug("deleted tenant config",
		zap.String("tenant_id", tenantID),
	)
}

func (w *Watcher) onPolicyUpsert(obj any) {
	pol, ok := obj.(*v1alpha1.Policy)
	if !ok || pol == nil {
		w.log.Warn("received non-Policy object in upsert handler; ignoring")
		return
	}

	tenantID := tenantIDFromNamespace(pol.Namespace)
	if tenantID == "" {
		w.log.Warn("policy missing namespace; ignoring",
			zap.String("resourceVersion", pol.ResourceVersion),
		)
	}

	w.applyPatch(tenantPatch{
		tenantID:    tenantID,
		authzPolicy: toAuthzPolicySnapshot(pol),
		revision:    pol.ResourceVersion,
		updated:     time.Now().UTC(),
	})

	w.log.Debug("upserted policy",
		zap.String("tenant_id", tenantID),
	)
}

func (w *Watcher) onPolicyDelete(obj any) {
	pol := extractPolicy(obj)
	if pol == nil {
		w.log.Warn("received unrecognised Policy delete object; ignoring")
		return
	}

	tenantID := tenantIDFromNamespace(pol.Namespace)
	if tenantID == "" {
		return
	}

	w.applyPatch(tenantPatch{
		tenantID:         tenantID,
		clearAuthzPolicy: true,
		revision:         pol.ResourceVersion,
		updated:          time.Now().UTC(),
	})

	w.log.Debug("deleted policy",
		zap.String("tenant_id", tenantID),
	)
}

func tenantCacheKey(t *v1alpha1.Tenant) string {
	if t == nil {
		return ""
	}

	if t.Spec.TenantID != "" {
		return t.Spec.TenantID
	}

	if t.Status.Namespace != "" {
		return strings.TrimPrefix(t.Status.Namespace, "tenant-")
	}

	return ""
}

func tenantIDFromNamespace(namesapce string) string {
	return strings.TrimPrefix(namesapce, "tenant-")
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

func toCloudResourcesMap(cr v1alpha1.CloudResources) map[string]any {
	out := map[string]any{}
	if cr.IdentityPoolID != "" {
		out["identityPoolID"] = cr.IdentityPoolID
	}

	if cr.IdentityPoolARN != "" {
		out["identityPoolARN"] = cr.IdentityPoolARN
	}

	if cr.StorageBucketName != "" {
		out["storageBucketName"] = cr.StorageBucketName
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func toAuthzPolicySnapshot(pol *v1alpha1.Policy) *app.AuthzPolicySnapshot {
	if pol == nil {
		return nil
	}

	out := &app.AuthzPolicySnapshot{
		IssuerAllowList: append([]string(nil), pol.Spec.JWT.IssuerAllowList...),
		SubjectClaim:    pol.Spec.JWT.SubjectClaim,
		ScopesClaim:     pol.Spec.JWT.ScopesClaim,
		Bindings:        make([]app.AuthzBindingSnapshot, 0, len(pol.Spec.Bindings)),
		RoleBindings:    make([]app.AuthzRoleBindingSnapshot, 0, len(pol.Spec.RoleBindings)),
	}

	for _, b := range pol.Spec.Bindings {
		out.Bindings = append(out.Bindings, app.AuthzBindingSnapshot{
			Subject:     b.Subject,
			Permissions: append([]string(nil), b.Permissions...),
		})
	}

	for _, rb := range pol.Spec.RoleBindings {
		out.RoleBindings = append(out.RoleBindings, app.AuthzRoleBindingSnapshot{
			Role:        rb.Role,
			Permissions: append([]string(nil), rb.Permissions...),
		})
	}

	return out
}

// extract* helpers handle both direct objects and DeletedFinalStateUnknow
// tombstones that the informer may deliver on delete events.

func extractTenant(obj any) *v1alpha1.Tenant {
	if t, ok := obj.(*v1alpha1.Tenant); ok {
		return t
	}

	inner := unwrapTombstone(obj)
	if t, ok := inner.(*v1alpha1.Tenant); ok {
		return t
	}

	return nil
}

func extractTenantConfig(obj any) *v1alpha1.TenantConfig {
	if tc, ok := obj.(*v1alpha1.TenantConfig); ok {
		return tc
	}

	return nil
}

func extractPolicy(obj any) *v1alpha1.Policy {
	if p, ok := obj.(*v1alpha1.Policy); ok {
		return p
	}

	inner := unwrapTombstone(obj)
	if p, ok := inner.(*v1alpha1.Policy); ok {
		return p
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
