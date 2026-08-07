package webhook

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewClient builds a writable Kubernetes client scoped to the webhook -
// serve's ClusterRole grants no write verbs, and kube.Watcher's cache is
// read-only by construction, so this stays out of internal/kube rather than
// widening what either shares. Uncached: SecretProviderClass is never a
// watched ConfigSource type, so there's no cache to route reads through.
func NewClient(restCfg *rest.Config, scheme *runtime.Scheme) (client.Client, error) {
	return client.New(restCfg, client.Options{Scheme: scheme})
}
