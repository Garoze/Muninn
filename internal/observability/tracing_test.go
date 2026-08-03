package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func TestNewTracerProvider_ServiceNameAttribute(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp, err := newTracerProvider(exp, 1.0)
	if err != nil {
		t.Fatalf("newTracerProvider: %v", err)
	}
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "test-span")
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	found := false
	for _, attr := range spans[0].Resource.Attributes() {
		if attr.Key == semconv.ServiceNameKey && attr.Value.AsString() == "muninn" {
			found = true
		}
	}
	if !found {
		t.Error("expected service.name=muninn resource attribute")
	}
}

// TestNewTracerProvider_ZeroRatioDropsUnparentedSpan establishes the
// baseline the next test depends on: with sampleRatio=0 and no parent, the
// TraceIDRatioBased sampler is actually wired (not a no-op) and drops the
// span.
func TestNewTracerProvider_ZeroRatioDropsUnparentedSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp, err := newTracerProvider(exp, 0)
	if err != nil {
		t.Fatalf("newTracerProvider: %v", err)
	}
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "unparented-span")
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	if got := len(exp.GetSpans()); got != 0 {
		t.Errorf("expected 0 recorded spans with sampleRatio=0 and no parent, got %d", got)
	}
}

// TestNewTracerProvider_ParentBasedHonorsSampledParent verifies the specific
// invariant config.go's TraceSampleRatio doc comment commits to: "Uses
// ParentBased wrapping so inbound sampled traces are always honored." A
// sampleRatio of 0 would drop an unparented span (previous test) but must
// NOT drop one whose incoming parent context was already marked sampled.
func TestNewTracerProvider_ParentBasedHonorsSampledParent(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp, err := newTracerProvider(exp, 0)
	if err != nil {
		t.Fatalf("newTracerProvider: %v", err)
	}
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	sampledParent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sampledParent)

	_, span := tp.Tracer("test").Start(ctx, "child-of-sampled-parent")
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("force flush: %v", err)
	}

	if got := len(exp.GetSpans()); got != 1 {
		t.Errorf("expected the span to be recorded despite sampleRatio=0, since its parent was already sampled; got %d spans", got)
	}
}
