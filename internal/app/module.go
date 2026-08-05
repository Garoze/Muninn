package app

import "go.uber.org/fx"

var Module = fx.Options(
	fx.Provide(NewDiscoveryService),
	// Exposes *Cache directly for other Fx consumers, backed by the same
	// instance the service owns.
	fx.Provide(func(s *DiscoveryService) *Cache { return s.Cache }),
)
