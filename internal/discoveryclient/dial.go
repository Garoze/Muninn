package discoveryclient

import (
	"fmt"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial opens a gRPC connection to addr - the caller owns closing the
// returned conn. TLS is opt-in via caPath, mirroring the server's own
// opt-in TLS posture (see transport/grpc.TLSServerOption): an empty
// caPath dials plaintext via insecure.NewCredentials(), matching the
// server's plaintext default and the assumption that a service mesh's
// mutual TLS (or an equivalent network layer) terminates transport
// security instead. A non-empty caPath verifies the server's certificate
// against that CA - required for every caller of this package (muninnctl,
// muninn resolve) once a deployment sets GRPC_TLS_CERT_PATH/
// GRPC_TLS_KEY_PATH on the server, since a plaintext client can't
// complete a TLS handshake against it.
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

// TLSDialOption builds a grpc.DialOption from caPath, mirroring
// TLSServerOption's opt-in shape on the client side: an empty caPath
// returns insecure.NewCredentials() (plaintext, today's default), a
// non-empty one loads caPath as a trust root and verifies the server's
// certificate against it. There's no client certificate involved - the
// gRPC API's direct TLS is one-way (see TLSServerOption), not mutual;
// mTLS is a service mesh's job when one is present, not this client's -
// so the client side only ever needs a CA to verify with, never a
// cert/key pair of its own.
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
