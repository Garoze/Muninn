package discoveryclient

import (
	"testing"
)

// grpc.NewClient never errors synchronously regardless of target validity
// (confirmed by direct probing against this grpc-go version) - connection
// establishment, including target parsing, is fully deferred to the first
// RPC. So there's no "invalid target returns an error from Dial" case to
// test: Dial's error return only ever fires if grpc.NewClient's signature
// changes to validate eagerly in a future version.
func TestDial_ReturnsUsableClientAndConn(t *testing.T) {
	client, conn, err := Dial("localhost:5010")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if client == nil {
		t.Error("expected a non-nil DiscoveryServiceClient")
	}
	if conn == nil {
		t.Error("expected a non-nil *grpc.ClientConn")
	}
}
