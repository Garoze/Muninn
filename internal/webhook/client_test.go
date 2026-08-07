package webhook

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	kubeModule "github.com/garoze/muninn/internal/kube"
)

func TestNewClient_ValidConfig_ReturnsUsableClient(t *testing.T) {
	scheme, err := kubeModule.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	c, err := NewClient(&rest.Config{Host: "https://127.0.0.1:6443"}, scheme)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned a nil client with no error")
	}
	// Confirms the scheme actually made it into the client, not just that
	// construction didn't error - a client built with the wrong/nil scheme
	// would still construct successfully but fail every real Get/Patch call.
	if !c.Scheme().Recognizes(corev1.SchemeGroupVersion.WithKind("ConfigMap")) {
		t.Error("client's scheme does not recognize ConfigMap - NewScheme's result wasn't actually wired in")
	}
}

func TestNewClient_InvalidHost_Errors(t *testing.T) {
	scheme, err := kubeModule.NewScheme()
	if err != nil {
		t.Fatalf("new scheme: %v", err)
	}

	// A Host containing a control character is rejected by net/url at
	// client construction time, before any network call - the one
	// reachable error path client.New has without a live server to dial.
	_, err = NewClient(&rest.Config{Host: "http://\x7f"}, scheme)
	if err == nil {
		t.Fatal("expected an error for an invalid Host, got nil")
	}
}
