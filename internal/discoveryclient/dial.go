package discoveryclient

import (
	"fmt"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial opens a plain-text gRPC connection to addr - the caller owns
// closing the returned conn.
//
// insecure.NewCredentials() is intentional, for every caller of this
// package (muninnctl, muninn resolve): network-level access to Muninn's
// API is the only access control in effect. A real deployment terminates
// transport security at a service mesh's mutual TLS or an equivalent
// network layer - not inside this client - so plaintext here is correct
// whether or not a mesh is present, not an assumption that one is.
func Dial(addr string) (discoveryv1.DiscoveryServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	return discoveryv1.NewDiscoveryServiceClient(conn), conn, nil
}
