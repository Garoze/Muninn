package app

import "go.uber.org/fx"

var Module = fx.Options(
	fx.Provide(NewDiscoveryService),
	// Re-expose *Cache to kube.Watcher gets the same instance the service owns.
	fx.Provide(func(s *DiscoveryService) *Cache { return s.Cache }),
)
