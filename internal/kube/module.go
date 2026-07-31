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

	"github.com/garoze/muninn/internal/config"
)

var Module = fx.Options(
	fx.Provide(newRestConfig),
	fx.Provide(NewScheme),
	fx.Provide(NewWatcher),
	fx.Invoke(initControllerRuntimeLogger),
	fx.Invoke(startWatcher),
)

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
