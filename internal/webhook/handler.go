package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

// Handler serves the mutating webhook's /mutate endpoint.
type Handler struct {
	log     *zap.Logger
	cfg     *config.Config
	metrics *observability.Metrics
	svc     *app.DiscoveryService
	client  client.Client
}

func NewHandler(log *zap.Logger, cfg *config.Config, metrics *observability.Metrics, svc *app.DiscoveryService, c client.Client) *Handler {
	return &Handler{log: log.Named("webhook"), cfg: cfg, metrics: metrics, svc: svc, client: c}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	outcome := "allowed"
	defer func() {
		h.metrics.WebhookRequestsTotal.WithLabelValues(outcome).Inc()
		h.metrics.WebhookRequestDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
	}()

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		outcome = "error"
		h.log.Error("failed to decode AdmissionReview",
			zap.Error(err),
		)
		http.Error(w, "invalid AdmissionReview", http.StatusBadRequest)
		return
	}

	if review.Request == nil {
		outcome = "error"
		http.Error(w, "AdmissionReview.Request is nil", http.StatusBadRequest)
		return
	}

	resp := &admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	// Fail open: failurePolicy: Fail means a decode error here would block
	// admission for every Pod in the cluster, not just annotated ones.
	var pod corev1.Pod
	if err := json.Unmarshal(review.Request.Object.Raw, &pod); err != nil {
		h.log.Warn("failed to decode Pod from AdmissionRequest, skipping injection",
			zap.Error(err),
		)
	} else if ShouldInject(&pod) {
		// The webhook's own cache is separate from server's - not-yet-synced
		// degrades to skip injection rather than blocking admission, same
		// fail-open precedent as the decode-failure branch above.
		if !h.svc.Cache.IsSynced() {
			h.log.Warn("webhook's own config cache not yet synced, skipping injection",
				zap.String("namespace", review.Request.Namespace),
			)
		} else {
			resolved := resolvedValues(r, h.svc, review.Request.Namespace, h.log)
			refs := ExtractSecretRefs(resolved, h.log)

			// Unlike the fail-open handling above, a SecretProviderClass
			// the webhook couldn't provision/validate fails admission
			// closed - the operator declared a secret reference Muninn
			// can't stand behind, rather than a best-effort feature
			// degrading silently. outcome is "denied", not "error": this is
			// a correct Allowed:false decision, not a webhook malfunction -
			// conflating the two would hide real webhook errors behind
			// legitimate policy denials in WebhookRequestsTotal.
			dryRun := review.Request.DryRun != nil && *review.Request.DryRun

			if len(refs) > 0 {
				if err := ReconcileSecretProviderClass(r.Context(), h.client, h.cfg, review.Request.Namespace, refs, dryRun); err != nil {
					outcome = "denied"
					h.log.Error("failed to reconcile SecretProviderClass, denying admission",
						zap.String("namespace", review.Request.Namespace),
						zap.Error(err),
					)
					resp.Allowed = false
					resp.Result = &metav1.Status{Message: err.Error()}
					if err := writeAdmissionReview(w, &review, resp); err != nil {
						outcome = "error"
						h.log.Error("failed to encode AdmissionReview response", zap.Error(err))
					}
					return
				}
			}

			ops := BuildPatch(&pod, review.Request.Namespace, h.cfg, refs)
			if len(ops) > 0 {
				patchBytes, err := json.Marshal(ops)
				if err != nil {
					outcome = "error"
					h.log.Error("failed to marshal patch",
						zap.Error(err),
					)
					http.Error(w, "failed to build patch", http.StatusInternalServerError)
					return
				}

				h.metrics.WebhookInjectionsTotal.Inc()

				patchType := admissionv1.PatchTypeJSONPatch
				resp.Patch = patchBytes
				resp.PatchType = &patchType

				h.log.Info("injecting config volume/containers",
					zap.String("namespace", review.Request.Namespace),
					zap.String("pod", pod.GetName()),
					zap.Int("ops", len(ops)),
				)

			}
		}
	}

	if err := writeAdmissionReview(w, &review, resp); err != nil {
		outcome = "error"
		h.log.Error("failed to encode AdmissionReview response", zap.Error(err))
	}
}

// writeAdmissionReview encodes resp as the response to review's request.
func writeAdmissionReview(w http.ResponseWriter, review *admissionv1.AdmissionReview, resp *admissionv1.AdmissionResponse) error {
	out := &admissionv1.AdmissionReview{
		TypeMeta: review.TypeMeta,
		Response: resp,
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(out)
}

// resolvedValues resolves a namespace's configuration against the webhook's
// own in-process cache - no gRPC call to server, see docs/design.md. An
// unconfigured namespace (no ConfigMap yet) is a normal case, not a failure;
// only a genuine resolve error is logged.
func resolvedValues(r *http.Request, svc *app.DiscoveryService, namespace string, log *zap.Logger) map[string]any {
	results, _, err := svc.Resolve(r.Context(), namespace)
	if err != nil && !errors.Is(err, app.ErrNamespaceNotFound) {
		log.Warn("failed to resolve namespace configuration",
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return nil
	}

	out := make(map[string]any, len(results))
	for _, res := range results {
		out[res.Key] = res.Value
	}

	return out
}
