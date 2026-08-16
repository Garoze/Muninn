package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/zap"
	health "google.golang.org/grpc/health"

	appModule "github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	kubeModule "github.com/garoze/muninn/internal/kube"
	"github.com/garoze/muninn/internal/observability"
	grpcTransport "github.com/garoze/muninn/internal/transport/grpc"
	"github.com/garoze/muninn/internal/version"
	webhookModule "github.com/garoze/muninn/internal/webhook"
)

// startServe wires the resolver: ConfigSource watchers, in-memory cache,
// and the gRPC Query/Describe API. This is "muninn server".
func startServe() (*fx.App, error) {
	app := fx.New(
		fx.Provide(config.New),
		observability.Module,
		appModule.Module,
		kubeModule.Module,
		grpcTransport.Module,

		fx.Invoke(func(lc fx.Lifecycle, log *zap.Logger) {
			lc.Append(fx.Hook{
				OnStop: func(context.Context) error {
					log.Info("shutting down")
					return nil
				},
			})
		}),
	)

	cfg, err := config.New()
	if err != nil {
		return nil, err
	}
	startCtx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return nil, err
	}

	return app, nil
}

// startWebhook wires the mutating admission webhook HTTP server. Constructors
// are called directly instead of the full observability.Module, which also
// brings pieces webhook mode doesn't need.
//
// appModule.Module and kubeModule.Module give the webhook its own
// informer/cache, separate from server's - the webhook resolves config
// in-process rather than dialing server's gRPC API, so admission latency and
// availability never depend on server being reachable. grpcTransport.Module
// is deliberately not included: webhook mode runs no gRPC server of its own.
func startWebhook() (*fx.App, error) {
	app := fx.New(
		fx.Provide(config.New),
		fx.Provide(observability.NewLogger),
		fx.Provide(observability.NewTracerProvider),
		fx.Invoke(observability.ShutdownTracerProvider),
		fx.Provide(func() prometheus.Registerer { return prometheus.DefaultRegisterer }),
		fx.Provide(observability.NewMetrics),
		fx.Invoke(observability.StartMetricsServer),
		// kube.Watcher's health-server params exist to gate serve's gRPC
		// readiness; webhook mode has no gRPC server to gate, so both are nil
		// - MarkHealthServing already nil-checks each independently.
		fx.Provide(func() *health.Server { return nil }),
		fx.Provide(func() *observability.StandaloneHealth { return nil }),
		appModule.Module,
		kubeModule.Module,
		webhookModule.Module,
	)

	cfg, err := config.New()
	if err != nil {
		return nil, err
	}
	startCtx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return nil, err
	}

	return app, nil
}

// printUsage writes the top-level command overview to w. Write errors are
// discarded - nothing useful to do with one this close to exit.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `muninn is the runtime configuration resolver server.

Commands:
  serve      Watch config sources and serve the gRPC Query/Describe/Resolve API
  webhook    Run the mutating admission webhook server
  resolve    Resolve a namespace once (or with --watch) and write it to a file
  version    Print the version and exit

Usage:
  muninn [command] [flags]

Use "muninn <command> --help" for more information about a given command.
`)
}

func run() (*fx.App, error) {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		os.Exit(0)
	case "serve":
		return startServe()
	case "webhook":
		return startWebhook()
	case "version":
		_, _ = fmt.Fprintln(os.Stdout, version.Version)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "[muninn] unknown command: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}

	return nil, nil // unreachable
}

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "resolve" {
		if err := cmdResolve(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "[muninn] %v\n", err)
			os.Exit(1)
		}
		return
	}

	app, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] failed to start: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.Stop(stopCtx); err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] failed to stop cleanly: %v\n", err)
		os.Exit(1)
	}
}
