package webhook

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
)

// Handler serves the mutating webhook's /mutate endpoint.
type Handler struct {
	log *zap.Logger
}

func NewHandler(log *zap.Logger) *Handler {
	return &Handler{log: log.Named("webhook")}
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

	// Stub: allow every request unmodified. Real path logic (idempotency
	//check + valoume/init-container/sidecar injection) lands in the next step.
	resp := &admissionv1.AdmissionReview{
		TypeMeta: review.TypeMeta,
		Response: &admissionv1.AdmissionResponse{
			UID:     review.Request.UID,
			Allowed: true,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("failed to encode AdmissionReview response",
			zap.Error(err),
		)
	}
}
