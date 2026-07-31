package observability

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func checkStatus(t *testing.T, hs *health.Server) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := hs.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return resp.Status
}

func TestRegisterGRPCHealth_StartsNotServing(t *testing.T) {
	s := grpc.NewServer()
	hs := RegisterGRPCHealth(s)

	if got := checkStatus(t, hs); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("got %v, want NOT_SERVING", got)
	}
}

func TestMarkHealthServing_FlipsMainToServing(t *testing.T) {
	s := grpc.NewServer()
	mainHS := RegisterGRPCHealth(s)

	MarkHealthServing(mainHS, nil)

	if got := checkStatus(t, mainHS); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("got %v, want SERVING", got)
	}
}

func TestMarkHealthServing_FlipsBothMainAndProbe(t *testing.T) {
	s := grpc.NewServer()
	mainHS := RegisterGRPCHealth(s)
	probeHS := &StandaloneHealth{Server: health.NewServer()}
	probeHS.Server.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	MarkHealthServing(mainHS, probeHS)

	if got := checkStatus(t, mainHS); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("main: got %v, want SERVING", got)
	}
	if got := checkStatus(t, probeHS.Server); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("probe: got %v, want SERVING", got)
	}
}

func TestMarkHealthServing_NilMainHSIsSafe(t *testing.T) {
	probeHS := &StandaloneHealth{Server: health.NewServer()}
	MarkHealthServing(nil, probeHS) // must not panic

	if got := checkStatus(t, probeHS.Server); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("got %v, want SERVING", got)
	}
}

func TestMarkHealthServing_NilProbeHSIsSafe(t *testing.T) {
	s := grpc.NewServer()
	mainHS := RegisterGRPCHealth(s)
	MarkHealthServing(mainHS, nil) // must not panic

	if got := checkStatus(t, mainHS); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("got %v, want SERVING", got)
	}
}

func TestMarkHealthServing_NilProbeHSServerIsSafe(t *testing.T) {
	s := grpc.NewServer()
	mainHS := RegisterGRPCHealth(s)
	probeHS := &StandaloneHealth{Server: nil}

	MarkHealthServing(mainHS, probeHS) // must not panic despite nil inner Server

	if got := checkStatus(t, mainHS); got != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("got %v, want SERVING", got)
	}
}

func TestMarkHealthServing_BothNilIsSafe(t *testing.T) {
	MarkHealthServing(nil, nil) // must not panic
}
