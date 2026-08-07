package grpc

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/observability"
)

// DiscoveryHandler adapts gRPC requests to the domain DiscoveryService.
// It is responsible only for protocol translation - no business logic lives here.
type DiscoveryHandler struct {
	Service *app.DiscoveryService
	Metrics *observability.Metrics
	Logger  *zap.Logger

	discoveryv1.UnimplementedDiscoveryServiceServer
}

const queryMethod = "/discovery.v1.DiscoveryService/Query"

// Query translates a gRPC QueryRequest into a domain Query call,
// records metrics and logs, and maps results back to proto types.
func (h *DiscoveryHandler) Query(ctx context.Context, req *discoveryv1.QueryRequest) (*discoveryv1.QueryResponse, error) {
	start := time.Now()

	results, missing, revision, err := h.Service.Query(ctx, req.Namespace, req.Keys, req.Strict)

	h.Metrics.RequestDuration.WithLabelValues("query").Observe(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, app.ErrCacheEntryStale) {
			h.Metrics.CacheStaleRejectionTotal.Inc()
		}

		c := classifyError(err)

		h.Metrics.RequestsTotal.WithLabelValues("query", c.resultLabel, c.codeLabel).Inc()
		h.Logger.Error("query failed",
			zap.String("method", queryMethod),
			zap.String("namespace", req.Namespace),
			zap.Int("keys_count", len(req.Keys)),
			zap.String("grpc_code", c.codeLabel),
			zap.Error(err),
		)
		return nil, status.Error(c.grpcCode, err.Error())
	}

	values := make([]*discoveryv1.KeyValue, 0, len(results))
	for _, r := range results {
		pbValue, convErr := structpb.NewValue(r.Value)
		if convErr != nil {
			h.Metrics.RequestsTotal.WithLabelValues("query", "internal", "Internal").Inc()
			h.Logger.Error("failed to serialize query results",
				zap.String("method", queryMethod),
				zap.String("namespace", req.Namespace),
				zap.String("key", r.Key),
				zap.Error(convErr),
			)
			return nil, status.Error(codes.Internal, "failed to serialize query value")
		}

		values = append(values, &discoveryv1.KeyValue{
			Key:    r.Key,
			Value:  pbValue,
			Source: r.Source,
		})
	}

	h.Metrics.RequestsTotal.WithLabelValues("query", "success", "OK").Inc()
	h.Logger.Info("query completed",
		zap.String("method", queryMethod),
		zap.String("namespace", req.Namespace),
		zap.Int("keys_count", len(req.Keys)),
	)

	return &discoveryv1.QueryResponse{
		Values:      values,
		MissingKeys: missing,
		Revision:    revision,
	}, nil
}

// Describe returns the active config sources. Source of truth is the
// app-layer DiscoveryService.Describe().
func (h *DiscoveryHandler) Describe(_ context.Context, _ *discoveryv1.DescribeRequest) (*discoveryv1.DescribeResponse, error) {
	sources := h.Service.Describe()

	resp := &discoveryv1.DescribeResponse{
		Sources: make([]*discoveryv1.ConfigSource, 0, len(sources)),
	}

	for _, s := range sources {
		resp.Sources = append(resp.Sources, &discoveryv1.ConfigSource{
			Kind:          s.Kind,
			LabelSelector: s.LabelSelector,
			Scope:         s.Scope,
		})
	}
	return resp, nil
}

const resolveMethod = "/discovery.v1.DiscoveryService/Resolve"

// Resolve translates a gRPC ResolveRequest into a domain Resolve call,
// records metrics and logs, and maps results back to proto types. Unlike
// Query, it takes no keys and returns everything resolved for the namespace.
func (h *DiscoveryHandler) Resolve(ctx context.Context, req *discoveryv1.ResolveRequest) (*discoveryv1.ResolveResponse, error) {
	start := time.Now()

	results, revision, err := h.Service.Resolve(ctx, req.Namespace)

	h.Metrics.RequestDuration.WithLabelValues("resolve").Observe(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, app.ErrCacheEntryStale) {
			h.Metrics.CacheStaleRejectionTotal.Inc()
		}

		c := classifyError(err)

		h.Metrics.RequestsTotal.WithLabelValues("resolve", c.resultLabel, c.codeLabel).Inc()
		h.Logger.Error("resolve failed",
			zap.String("method", resolveMethod),
			zap.String("namespace", req.Namespace),
			zap.String("grpc_code", c.codeLabel),
			zap.Error(err),
		)

		return nil, status.Error(c.grpcCode, err.Error())
	}

	values := make([]*discoveryv1.KeyValue, 0, len(results))
	for _, r := range results {
		pbValue, convErr := structpb.NewValue(r.Value)
		if convErr != nil {
			h.Metrics.RequestsTotal.WithLabelValues("resolve", "internal", "Internal").Inc()
			h.Logger.Error("failed to serialize resolve results",
				zap.String("method", resolveMethod),
				zap.String("namespace", req.Namespace),
				zap.String("key", r.Key),
				zap.Error(convErr),
			)

			return nil, status.Error(codes.Internal, "failed to serialize resolve value")
		}

		values = append(values, &discoveryv1.KeyValue{
			Key:    r.Key,
			Value:  pbValue,
			Source: r.Source,
		})
	}

	h.Metrics.RequestsTotal.WithLabelValues("resolve", "success", "OK").Inc()
	h.Logger.Info("resolve completed",
		zap.String("method", resolveMethod),
		zap.String("namespace", req.Namespace),
		zap.Int("keys_resolved", len(values)),
	)

	return &discoveryv1.ResolveResponse{
		Values:   values,
		Revision: revision,
	}, nil
}

// classifyError maps domain sentinel errors to gRPC status codes and metric labels.
// Uses error.Is because app layer wraps sentinels with fmt.Errorf("%w", ...).
type errorClassification struct {
	resultLabel string
	codeLabel   string
	grpcCode    codes.Code
}

func classifyError(err error) errorClassification {
	switch {
	case errors.Is(err, app.ErrNamespaceNotFound):
		return errorClassification{"not_found", "NotFound", codes.NotFound}
	case errors.Is(err, app.ErrNamespaceRequired),
		errors.Is(err, app.ErrStrictMissingKeys):
		return errorClassification{"invalid_argument", "InvalidArgument", codes.InvalidArgument}
	case errors.Is(err, app.ErrCacheNotSynced),
		errors.Is(err, app.ErrCacheEntryStale):
		return errorClassification{"unavailable", "Unavailable", codes.Unavailable}
	default:
		return errorClassification{"internal", "Internal", codes.Internal}
	}
}
