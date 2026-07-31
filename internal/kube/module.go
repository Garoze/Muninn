package kube

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/garoze/muninn/internal/config"
)

var Module = fx.Options(
	fx.Provide(newRestConfig),
	fx.Provide(NewScheme),
	fx.Provide(NewWatcher),
	fx.Invoke(startWatcher),
)

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
