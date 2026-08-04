package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"

	appModule "github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	kubeModule "github.com/garoze/muninn/internal/kube"
	"github.com/garoze/muninn/internal/observability"
	grpcTransport "github.com/garoze/muninn/internal/transport/grpc"
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

		// gRPC server - constructed here (composition root) to avoid an
		// import cycle between observability (listener/health) and
		// transport/grpc (handler registration). Depends on *sdktrace.TracerProvider
		// explicitly (not otel.GetTracerProvider()'s global) so Fx sequences
		// NewTracerProvider before this runs - otelgrpc.NewServerHandler resolves
		// its TracerProvider once at construction, not lazily per request.
		fx.Provide(func(log *zap.Logger, tp *sdktrace.TracerProvider) (*grpc.Server, *health.Server) {
			r := observability.NewGRPCServer(log,
				grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tp))),
			)
			return r.Server, r.HealthServer
		}),

		fx.Invoke(func(lc fx.Lifecycle, log *zap.Logger) {
			lc.Append(fx.Hook{
				OnStop: func(context.Context) error {
					log.Info("shutting down")
					return nil
				},
			})
		}),
	)

	cfg := config.New()
	startCtx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return nil, err
	}

	return app, nil
}

// startWebhook wires the mutating admission webhook HTTP server. This is
// "muninn webhook". Like NewLogger, NewTracerProvider is called directly
// rather than pulling in the whole observability.Module - that bundle also
// brings NewMetrics/startMetricsServer/NewGRPCListener, none of which
// webhook mode needs.
func startWebhook() (*fx.App, error) {
	app := fx.New(
		fx.Provide(config.New),
		fx.Provide(observability.NewLogger),
		fx.Provide(observability.NewTracerProvider),
		fx.Invoke(observability.ShutdownTracerProvider),
		webhookModule.Module,
	)

	cfg := config.New()
	startCtx, cancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return nil, err
	}

	return app, nil
}

// printUsage writes the top-level command overview to w. The write error is
// deliberately discarded: this is help text printed immediately before the
// program exits or returns, so a failed write (e.g. closed stdout pipe)
// isn't something the CLI could act on differently.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `muninn is the runtime configuration resolver server.

Commands:
  serve      Watch config sources and serve the gRPC Query/Describe/Resolve API
  webhook    Run the mutating admission webhook server
  resolve    Resolve a namespace once (or with --watch) and write it to a file

Usage:
  muninn [command]

Use "muninn resolve -h" for resolve's flags.
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
