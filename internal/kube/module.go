package kube

import (
	"context"
	"fmt"

	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
)

var Module = fx.Options(
	fx.Provide(newRestConfig),
	fx.Provide(NewScheme),
	// A bring-your-own ConfigSource is one more fx.Provide into this group,
	// with no change to NewWatcher or anything downstream of it.
	fx.Provide(
		fx.Annotate(NewConfigMapSource,
			fx.As(new(ConfigSource)),
			fx.ResultTags(`group:"config_sources"`),
		),
	),
	// Sits between the raw group and its consumers so Describe reports only
	// enabled sources, not just registered ones.
	fx.Provide(
		fx.Annotate(filterEnabledSources, fx.ParamTags("", `group:"config_sources"`)),
	),
	fx.Provide(NewWatcher),
	fx.Provide(provideConfigSourceDescriptors),
	fx.Invoke(initControllerRuntimeLogger),
	fx.Invoke(startWatcher),
)

// filterEnabledSources returns only the sources whose Kind() is named in
// cfg.EnabledConfigSources, or every registered source when that list is
// empty (the default - nothing filtered).
func filterEnabledSources(cfg *config.Config, sources []ConfigSource) []ConfigSource {
	if len(cfg.EnabledConfigSources) == 0 {
		return sources
	}

	enabled := make(map[string]bool, len(cfg.EnabledConfigSources))
	for _, kind := range cfg.EnabledConfigSources {
		enabled[kind] = true
	}

	out := make([]ConfigSource, 0, len(sources))
	for _, s := range sources {
		if enabled[s.Kind()] {
			out = append(out, s)
		}
	}
	return out
}

// provideConfigSourceDescriptors translates ConfigSources into
// app.ConfigSourceDescriptor. Lives here, not internal/app, since
// internal/app must not import controller-runtime types.
func provideConfigSourceDescriptors(sources []ConfigSource) []app.ConfigSourceDescriptor {
	out := make([]app.ConfigSourceDescriptor, 0, len(sources))
	for _, s := range sources {
		out = append(out, app.ConfigSourceDescriptor{
			Kind:          s.Kind(),
			LabelSelector: s.LabelSelector(),
			Scope:         s.Scope(),
		})
	}
	return out
}

// initControllerRuntimeLogger bridges the zap logger into controller-runtime's
// logr interface. Must run before any controller-runtime component (cache,
// informers) starts, otherwise it falls back to a no-op logger and warns.
func initControllerRuntimeLogger(log *zap.Logger) {
	crlog.SetLogger(zapr.NewLogger(log.Named("controller-runtime")))
}

func newRestConfig(cfg *config.Config) (*rest.Config, error) {
	if cfg.KubeConfigPath != "" {
		rc, err := clientcmd.BuildConfigFromFlags("", cfg.KubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("building kubeconfig from %s: %w", cfg.KubeConfigPath, err)
		}
		return rc, nil
	}

	rc, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("building in-cluster config: %w", err)
	}
	return rc, nil
}

func startWatcher(lc fx.Lifecycle, w *Watcher, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return w.Start(ctx)
		},
		OnStop: func(context.Context) error {
			w.Stop()
			return nil
		},
	})
}
