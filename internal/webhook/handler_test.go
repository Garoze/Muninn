package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/garoze/muninn/internal/config"
	"github.com/garoze/muninn/internal/observability"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, _ := newTestHandlerWithMetrics(t)
	return h
}

func newTestHandlerWithMetrics(t *testing.T) (*Handler, *observability.Metrics) {
	t.Helper()
	m := observability.NewMetrics(prometheus.NewRegistry())
	return NewHandler(zap.NewNop(), &config.Config{}, m), m
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

// TestServeHTTP_MissingObject_AllowsUnmodified covers a request with no
// "object" field (Object.Raw empty) - this webhook runs against every Pod
// create in the cluster under failurePolicy: Fail, so a decode failure here
// must not block admission for unrelated Pods; it should just skip
// injection and allow.
func TestServeHTTP_MissingObject_AllowsUnmodified(t *testing.T) {
	h := newTestHandler(t)

	body := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"no-object"}}`
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
	if review.Response == nil || !review.Response.Allowed {
		t.Fatalf("expected Allowed: true, got %+v", review.Response)
	}
	if review.Response.Patch != nil {
		t.Errorf("expected no patch, got %s", review.Response.Patch)
	}
}

func admissionReviewBody(t *testing.T, uid, namespace string, pod *corev1.Pod) []byte {
	t.Helper()

	podJSON, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}

	review := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       types.UID(uid),
			Namespace: namespace,
			Object:    runtime.RawExtension{Raw: podJSON},
		},
	}

	out, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal review: %v", err)
	}
	return out
}

func TestServeHTTP_AnnotatedPod_InjectsPatch(t *testing.T) {
	h := newTestHandler(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBody(t, "annotated", "ns1", pod)

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: body=%s", rec.Code, rec.Body.String())
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || !review.Response.Allowed {
		t.Fatalf("expected Allowed: true, got %+v", review.Response)
	}
	if review.Response.Patch == nil {
		t.Fatal("expected a non-nil patch for an annotated Pod")
	}
	if review.Response.PatchType == nil || *review.Response.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Errorf("PatchType: got %v, want JSONPatch", review.Response.PatchType)
	}

	// Decode the actual patch content, not just its presence - this is
	// what would have caught the addVolumeOp/addInitContainerOp bugs found
	// via manual curl verification (both applied cleanly with Patch != nil,
	// but the volume was silently empty and the initContainers path was
	// wrong).
	var ops []patchOperation
	if err := json.Unmarshal(review.Response.Patch, &ops); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	byPath := opsByPath(ops)

	volOp, ok := byPath["/spec/volumes"]
	if !ok {
		t.Fatal("missing /spec/volumes op")
	}
	var vols []corev1.Volume
	if b, err := json.Marshal(volOp.Value); err != nil || json.Unmarshal(b, &vols) != nil || len(vols) != 1 || vols[0].Name != volumeName {
		t.Errorf("volumes op value: got %+v", volOp.Value)
	}

	initOp, ok := byPath["/spec/initContainers"]
	if !ok {
		t.Fatal("missing /spec/initContainers op")
	}
	var initContainers []corev1.Container
	if b, err := json.Marshal(initOp.Value); err != nil || json.Unmarshal(b, &initContainers) != nil || len(initContainers) != 1 || initContainers[0].Name != initContainerName {
		t.Errorf("initContainers op value: got %+v", initOp.Value)
	}

	if _, ok := byPath["/spec/containers/0/volumeMounts"]; !ok {
		t.Error("missing /spec/containers/0/volumeMounts op (app container mount)")
	}
}

func TestServeHTTP_UnannotatedPod_NoPatch(t *testing.T) {
	h := newTestHandler(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBody(t, "unannotated", "ns1", pod)

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || !review.Response.Allowed {
		t.Fatalf("expected Allowed: true, got %+v", review.Response)
	}
	if review.Response.Patch != nil {
		t.Errorf("expected no patch for an unannotated Pod, got %s", review.Response.Patch)
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

func TestServeHTTP_RecordsMetrics(t *testing.T) {
	h, m := newTestHandlerWithMetrics(t)

	allowedBody := `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview","request":{"uid":"x"}}`
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader(allowedBody)))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader("not json")))

	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("allowed")); got != 1 {
		t.Errorf("allowed count: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("error")); got != 1 {
		t.Errorf("error count: got %v, want 1", got)
	}
	if got := testutil.CollectAndCount(m.WebhookRequestDuration); got == 0 {
		t.Error("expected WebhookRequestDuration to have recorded at least one observation")
	}
}

func TestServeHTTP_AnnotatedPod_RecordsInjection(t *testing.T) {
	h, m := newTestHandlerWithMetrics(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBody(t, "annotated", "ns1", pod)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body)))

	if got := testutil.ToFloat64(m.WebhookInjectionsTotal); got != 1 {
		t.Errorf("injections count: got %v, want 1", got)
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
