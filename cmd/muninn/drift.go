package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/garoze/muninn/internal/webhook"
)

// refKeys returns the *_ref-suffixed keys in resolved, sorted for
// deterministic comparison across polls.
func refKeys(resolved map[string]any) []string {
	var keys []string
	for k := range resolved {
		if strings.HasSuffix(k, webhook.RefKeySuffix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// newlyAppearedRefs returns the keys present in current but not previous.
// Callers must not invoke this against the very first poll's result -
// there's no prior state to have drifted from yet.
func newlyAppearedRefs(previous, current []string) []string {
	prevSet := make(map[string]bool, len(previous))
	for _, k := range previous {
		prevSet[k] = true
	}

	var added []string
	for _, k := range current {
		if !prevSet[k] {
			added = append(added, k)
		}
	}
	return added
}

// driftReporter surfaces newly-appeared *_ref keys the sidecar can't act on
// itself - CSI volumes are immutable post-mount, so picking up a new
// secret reference needs a Pod restart, an operator decision. Always logs;
// best-effort creates a Kubernetes Event on top of that. The sidecar runs
// under the consumer Pod's own ServiceAccount, not muninn-webhook's, and
// nothing in this design grants that ServiceAccount events.k8s.io
// create - a namespace that hasn't separately chosen to grant it still
// gets the log line instead of losing drift visibility entirely.
type driftReporter struct {
	once         sync.Once
	clientset    *kubernetes.Clientset
	podName      string
	podNamespace string
}

func (r *driftReporter) init() {
	r.podName = os.Getenv("POD_NAME")
	r.podNamespace = os.Getenv("POD_NAMESPACE")

	cfg, err := rest.InClusterConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] drift Event reporting disabled (not running in-cluster): %v\n", err)
		return
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] drift Event reporting disabled (building clientset): %v\n", err)
		return
	}
	r.clientset = cs
}

// Report logs unconditionally, then attempts a Kubernetes Event - see the
// type doc for why the Event half is best-effort, not required.
func (r *driftReporter) Report(ctx context.Context, namespace string, addedKeys []string) {
	fmt.Fprintf(os.Stderr,
		"[muninn] drift: new secret reference(s) %s appeared for namespace %s - restart this Pod to pick them up (CSI volumes are immutable once mounted)\n",
		strings.Join(addedKeys, ", "), namespace,
	)

	r.once.Do(r.init)
	if r.clientset == nil || r.podName == "" || r.podNamespace == "" {
		return
	}

	event := &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "muninn-secret-drift-",
			Namespace:    r.podNamespace,
		},
		EventTime:           metav1.NowMicro(),
		ReportingController: "muninn.io/resolve-sidecar",
		ReportingInstance:   r.podName,
		Action:              "NewSecretReferenceDetected",
		Reason:              "NewSecretReference",
		Regarding: corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: r.podNamespace,
			Name:      r.podName,
		},
		Note: fmt.Sprintf(
			"new secret reference(s) appeared in resolved config: %s - restart this Pod to pick them up (CSI volumes are immutable once mounted)",
			strings.Join(addedKeys, ", "),
		),
		Type: "Warning",
	}

	createCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := r.clientset.EventsV1().Events(r.podNamespace).Create(createCtx, event, metav1.CreateOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "[muninn] warning: failed to emit drift Event (log above still stands): %v\n", err)
	}
}
