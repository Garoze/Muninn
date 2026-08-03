package observability

import (
	"context"
	"fmt"

	"github.com/garoze/muninn/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NewTracerProvider creates an OTLP gRPC trace exporter and delegates to
// newTracerProvider. The caller (Fx) owns the provider's lifecycle - call
// Shutdown on stop.
func NewTracerProvider(cfg *config.Config) (*sdktrace.TracerProvider, error) {
	conn, err := grpc.NewClient(cfg.OTELExporterEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to OTLP endpoint %s: %w", cfg.OTELExporterEndpoint, err)
	}

	exp, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithGRPCConn(conn),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	return newTracerProvider(exp, cfg.TraceSampleRatio)
}

// newTracerProvider wires exp into a TracerProvider with a ParentBased
// sampler and the muninn service resource, and sets the OTel globals.
// Separated from NewTracerProvider so tests can inject an in-memory
// exporter instead of a real OTLP connection.
func newTracerProvider(exp sdktrace.SpanExporter, sampleRatio float64) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName("muninn")),
	)
	if err != nil {
		return nil, fmt.Errorf("building trace resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}
