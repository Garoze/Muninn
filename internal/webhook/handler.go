package webhook

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/garoze/muninn/internal/config"
)

// Handler serves the mutating webhook's /mutate endpoint.
type Handler struct {
	log *zap.Logger
	cfg *config.Config
}

func NewHandler(log *zap.Logger, cfg *config.Config) *Handler {
	return &Handler{log: log.Named("webhook"), cfg: cfg}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		h.log.Error("failed to decode AdmissionReview",
			zap.Error(err),
		)
		http.Error(w, "invalid AdmissionReview", http.StatusBadRequest)
		return
	}

	if review.Request == nil {
		http.Error(w, "AdmissionReview.Request is nil", http.StatusBadRequest)
		return
	}

	resp := &admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	// A decode failure here only ever means this webhook can't evaluate
	// injection for this one Pod - it never means the Pod itself is
	// invalid. With failurePolicy: Fail, this webhook runs against every
	// Pod create in the cluster, not just annotated ones, so failing
	// closed here would block admission for unrelated Pods over a
	// best-effort, opt-in feature. Log and allow unmodified instead.
	var pod corev1.Pod
	if err := json.Unmarshal(review.Request.Object.Raw, &pod); err != nil {
		h.log.Warn("failed to decode Pod from AdmissionRequest, skipping injection",
			zap.Error(err),
		)
	} else if ShouldInject(&pod) {
		ops := BuildPath(&pod, review.Request.Namespace, h.cfg)
		if len(ops) > 0 {
			patchBytes, err := json.Marshal(ops)
			if err != nil {
				h.log.Error("failed to marshal patch",
					zap.Error(err),
				)
				http.Error(w, "failed to build patch", http.StatusInternalServerError)
				return
			}

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

	out := &admissionv1.AdmissionReview{
		TypeMeta: review.TypeMeta,
		Response: resp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.log.Error("failed to encode AdmissionReview response",
			zap.Error(err),
		)
	}
}
