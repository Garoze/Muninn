package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(zap.NewNop())
}

func TestServeHTTP_ValidReview_AllowsAndEchoesUID(t *testing.T) {
	h := newTestHandler(t)

	body := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"test-uid-123"}}`
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: body=%s", rec.Code, rec.Body.String())
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil {
		t.Fatal("expected a non-nil Response")
	}
	if !review.Response.Allowed {
		t.Error("expected Allowed: true")
	}
	if string(review.Response.UID) != "test-uid-123" {
		t.Errorf("UID: got %q, want %q", review.Response.UID, "test-uid-123")
	}
}

func TestServeHTTP_MalformedJSON_ReturnsBadRequest(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestServeHTTP_NilRequest_ReturnsBadRequest(t *testing.T) {
	h := newTestHandler(t)

	// Well-formed AdmissionReview JSON, but no "request" field.
	body := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview"}`
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestServeHTTP_SetsJSONContentType(t *testing.T) {
	h := newTestHandler(t)

	body := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"x"}}`
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
}
