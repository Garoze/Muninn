package discoveryclient

import (
	"fmt"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial opens a gRPC connection to addr; the caller owns closing the
// returned conn. TLS is opt-in via caPath: empty dials plaintext,
// non-empty verifies the server's certificate against that CA.
func Dial(addr, caPath string) (discoveryv1.DiscoveryServiceClient, *grpc.ClientConn, error) {
	creds, err := TLSDialOption(caPath)
	if err != nil {
		return nil, nil, err
	}

	conn, err := grpc.NewClient(addr, creds)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	return discoveryv1.NewDiscoveryServiceClient(conn), conn, nil
}

// TLSDialOption returns plaintext credentials when caPath is empty, or
// credentials that verify the server's certificate against caPath
// otherwise. No client certificate is used - this API's TLS is one-way.
func TLSDialOption(caPath string) (grpc.DialOption, error) {
	if caPath == "" {
		return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
	}

	creds, err := credentials.NewClientTLSFromFile(caPath, "")
	if err != nil {
		return nil, fmt.Errorf("loading gRPC TLS CA %s: %w", caPath, err)
	}

	return grpc.WithTransportCredentials(creds), nil
}
