package grpc

import (
	"context"
	"errors"
	"sort"
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

	results, missing, revision, err := h.Service.Query(ctx, req.TenantId, req.Keys, req.Strict)

	h.Metrics.QueryDuration.WithLabelValues("query").Observe(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, app.ErrCacheEntryStale) {
			h.Metrics.CacheStaleRejectionTotal.Inc()
		}

		c := classifyError(err)

		h.Metrics.QueriesTotal.WithLabelValues(c.resultLabel, c.codelabel).Inc()
		h.Logger.Error("query failed",
			zap.String("method", queryMethod),
			zap.String("tenant_id", req.TenantId),
			zap.Int("keys_count", len(req.Keys)),
			zap.String("grpc_code", c.codelabel),
			zap.Error(err),
		)
		return nil, status.Error(c.grpcCode, err.Error())
	}

	values := make([]*discoveryv1.KeyValue, 0, len(results))
	for _, r := range results {
		pbValue, convErr := structpb.NewValue(r.Value)
		if convErr != nil {
			h.Metrics.QueriesTotal.WithLabelValues("internal", "Internal").Inc()
			h.Logger.Error("failed to serialize query results",
				zap.String("method", queryMethod),
				zap.String("tenant_id", req.TenantId),
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

	h.Metrics.QueriesTotal.WithLabelValues("success", "OK").Inc()
	h.Logger.Info("query completed",
		zap.String("method", queryMethod),
		zap.String("tenant_id", req.TenantId),
		zap.Int("keys_count", len(req.Keys)),
	)

	return &discoveryv1.QueryResponse{
		Values:      values,
		MissingKeys: missing,
		Revision:    revision,
	}, nil
}

// Describe returns all supported keys with type hints and descriptions,
// sorted alphabetically. Source of truth in the app-layer SupportedKeys map.
func (h *DiscoveryHandler) Describe(_ context.Context, _ *discoveryv1.DescribeRequest) (*discoveryv1.DescribeResponse, error) {
	keys := make([]string, 0, len(app.SupportedKeys))
	for k := range app.SupportedKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	resp := &discoveryv1.DescribeResponse{
		SupportedKeys: make([]*discoveryv1.SupportedKey, 0, len(keys)),
	}

	for _, k := range keys {
		desc, ok := app.SupportedKeyDescriptions[k]
		if !ok {
			desc = "no description provided"
		}

		resp.SupportedKeys = append(resp.SupportedKeys, &discoveryv1.SupportedKey{
			Key:         k,
			TypeHint:    app.SupportedKeys[k],
			Description: desc,
		})
	}
	return resp, nil
}

// classifyError maps domain sentinel errors to gRPC status codes and metric labels.
// Uses error.Is because app layer wraps sentinels with fmt.Errorf("%w", ...).
type errorClassification struct {
	resultLabel string
	codelabel   string
	grpcCode    codes.Code
}

func classifyError(err error) errorClassification {
	switch {
	case errors.Is(err, app.ErrTenantNotFound):
		return errorClassification{"not_found", "NotFound", codes.NotFound}
	case errors.Is(err, app.ErrUnsupportedKey),
		errors.Is(err, app.ErrTenantIDRequired),
		errors.Is(err, app.ErrStrictMissingKeys):
		return errorClassification{"invalid_argument", "InvalidArgument", codes.InvalidArgument}
	case errors.Is(err, app.ErrCacheNotSynced),
		errors.Is(err, app.ErrCacheEntryStale):
		return errorClassification{"unavailable", "Unavailable", codes.Unavailable}
	default:
		return errorClassification{"internal", "Internal", codes.Internal}
	}
}
