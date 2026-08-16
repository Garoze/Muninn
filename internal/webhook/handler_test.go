package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/garoze/muninn/internal/app"
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
	svc := app.NewDiscoveryService(&config.Config{}, zap.NewNop(), nil)
	svc.Cache.SetSynced()
	m := observability.NewMetrics(prometheus.NewRegistry())
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	return NewHandler(zap.NewNop(), &config.Config{}, m, svc, c), m
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
// "object" field - must skip injection and allow, not block admission.
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
	return admissionReviewBodyDryRun(t, uid, namespace, pod, false)
}

func admissionReviewBodyDryRun(t *testing.T, uid, namespace string, pod *corev1.Pod, dryRun bool) []byte {
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
			DryRun:    &dryRun,
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

	// Decode the actual patch content, not just its presence: a non-nil
	// patch can still carry an empty or malformed op value.
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
	if b, err := json.Marshal(initOp.Value); err != nil || json.Unmarshal(b, &initContainers) != nil ||
		len(initContainers) != 2 || initContainers[0].Name != initContainerName || initContainers[1].Name != sidecarContainerName {
		t.Errorf("initContainers op value: got %+v", initOp.Value)
	}
	// The watching container is a native sidecar, so it belongs here rather
	// than among the Pod's own containers, where it would keep an annotated
	// Job Pod from ever completing.
	if initContainers[1].RestartPolicy == nil || *initContainers[1].RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Error("sidecar must carry restartPolicy: Always")
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

func TestServerHTTP_CacheNotSynced_SkipInjectionButAllows(t *testing.T) {
	m := observability.NewMetrics(prometheus.NewRegistry())
	svc := app.NewDiscoveryService(&config.Config{}, zap.NewNop(), nil)
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h := NewHandler(zap.NewNop(), &config.Config{}, m, svc, c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}

	body := admissionReviewBody(t, "not-synced", "ni1", pod)

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

	if review.Response.Patch != nil {
		t.Errorf("expected no patch while cache is not synced, got %s", review.Response.Patch)
	}
}

// handlerWithSecretRef builds a Handler whose cache already has a resolved
// db_password_ref for namespace, so ServeHTTP reaches the
// ReconcileSecretProviderClass call rather than skipping it (empty refs).
func handlerWithSecretRef(t *testing.T, cfg *config.Config, namespace string, c client.Client) (*Handler, *observability.Metrics) {
	t.Helper()
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	svc.Cache.Set(&app.ConfigEntry{
		Namespace: namespace,
		Sources: map[string]map[string]any{
			"ConfigMap/app-config": {"db_password_ref": "vault://secret/data/prod/db-password"},
		},
	})
	svc.Cache.SetSynced()
	m := observability.NewMetrics(prometheus.NewRegistry())
	return NewHandler(zap.NewNop(), cfg, m, svc, c), m
}

func TestServeHTTP_ReconcileSucceeds_InjectsCSIVolume(t *testing.T) {
	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeCreate}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h, _ := handlerWithSecretRef(t, cfg, "prod", c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBody(t, "reconcile-ok", "prod", pod)

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
	if review.Response.Patch == nil {
		t.Fatal("expected a patch")
	}

	var ops []patchOperation
	if err := json.Unmarshal(review.Response.Patch, &ops); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	var vols []corev1.Volume
	for _, op := range ops {
		if op.Path != "/spec/volumes" {
			continue
		}
		b, _ := json.Marshal(op.Value)
		_ = json.Unmarshal(b, &vols)
	}
	found := false
	for _, v := range vols {
		if v.Name == csiVolumeName {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CSI secrets volume in patch, got volumes %+v", vols)
	}

	got := &secretsstorev1.SecretProviderClass{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "prod", Name: "muninn-secrets-prod"}, got); err != nil {
		t.Errorf("expected SecretProviderClass to be created by reconcile: %v", err)
	}
}

// The webhook registers sideEffects: NoneOnDryRun, so a server-side dry run
// must not create the SecretProviderClass. It must still return the patch it
// would have applied, or the dry run misreports the real mutation.
func TestServeHTTP_DryRun_CreateMode_WritesNothingButStillPatches(t *testing.T) {
	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeCreate}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h, _ := handlerWithSecretRef(t, cfg, "prod", c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBodyDryRun(t, "dry-run", "prod", pod, true)

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
	if review.Response.Patch == nil {
		t.Error("expected the patch to still be returned on a dry run")
	}

	got := &secretsstorev1.SecretProviderClass{}
	err := c.Get(context.Background(), client.ObjectKey{Namespace: "prod", Name: "muninn-secrets-prod"}, got)
	if err == nil {
		t.Fatal("dry run created a SecretProviderClass: sideEffects: NoneOnDryRun is violated")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

// Reference mode only reads, so a dry run still validates against the
// pre-provisioned object and still rejects a mismatch.
func TestServeHTTP_DryRun_ReferenceMode_StillDeniesOnMismatch(t *testing.T) {
	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeReference}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h, _ := handlerWithSecretRef(t, cfg, "prod", c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBodyDryRun(t, "dry-run-ref", "prod", pod, true)

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || review.Response.Allowed {
		t.Fatalf("expected the dry run to be denied for a missing pre-provisioned SPC, got %+v", review.Response)
	}
}

func TestServeHTTP_ReconcileFails_DeniesAdmission(t *testing.T) {
	// Reference mode with no pre-provisioned SPC: ReconcileSecretProviderClass
	// must return an error, and that must deny admission rather than the
	// fail-open pattern used elsewhere in this handler.
	cfg := &config.Config{SecretSPCMode: config.SecretSPCModeReference}
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h, m := handlerWithSecretRef(t, cfg, "prod", c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBody(t, "reconcile-fail", "prod", pod)

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 (admission response, not an HTTP error): body=%s", rec.Code, rec.Body.String())
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &review); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if review.Response == nil || review.Response.Allowed {
		t.Fatalf("expected Allowed: false, got %+v", review.Response)
	}
	if review.Response.Result == nil || review.Response.Result.Message == "" {
		t.Error("expected a Result.Message explaining the denial")
	}
	if review.Response.Patch != nil {
		t.Errorf("expected no patch on a denied admission, got %s", review.Response.Patch)
	}

	// "denied" is a distinct outcome from "error": this is a correct policy
	// decision, not a webhook malfunction, and the metric must say so.
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("denied")); got != 1 {
		t.Errorf("denied count: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("error")); got != 0 {
		t.Errorf("error count: got %v, want 0 (this is a denial, not an error)", got)
	}
}

// An annotated Pod whose configuration cannot be resolved must be denied, not
// admitted uninjected. Admitting it produces a Pod with neither its
// configuration nor - where the namespace carries secret references - its
// secrets mount, with nothing in the Pod to say so. The reconcile branch
// already refuses to degrade that way; this is the same case one step earlier.
func TestServeHTTP_ResolveFails_DeniesAdmission(t *testing.T) {
	// A stale cache entry is the reachable resolve failure here: ServeHTTP
	// gates on IsSynced before this point, so ErrCacheNotSynced cannot occur.
	cfg := &config.Config{CacheEntryTTL: time.Millisecond}
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	svc.Cache.Set(&app.ConfigEntry{
		Namespace: "prod",
		Sources:   map[string]map[string]any{"ConfigMap/x": {"LOG_LEVEL": "info"}},
		UpdatedAt: time.Now().Add(-time.Hour),
	})
	svc.Cache.SetSynced()

	m := observability.NewMetrics(prometheus.NewRegistry())
	c := fake.NewClientBuilder().WithScheme(testSPCScheme(t)).Build()
	h := NewHandler(zap.NewNop(), cfg, m, svc, c)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "example/app:latest"}},
		},
	}
	body := admissionReviewBody(t, "resolve-fail", "prod", pod)

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
	if review.Response == nil || review.Response.Allowed {
		t.Fatalf("expected Allowed: false, got %+v", review.Response)
	}
	if review.Response.Patch != nil {
		t.Errorf("expected no patch on a denied admission, got %s", review.Response.Patch)
	}
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("denied")); got != 1 {
		t.Errorf("denied count: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("error")); got != 0 {
		t.Errorf("error count: got %v, want 0 (a denial is not a malfunction)", got)
	}
}

// TestServeHTTP_AlreadyInjectedPod_NoPatchNoInjectionMetric proves
// idempotency end-to-end through ServeHTTP, not just at BuildPatch's own
// level (already covered in inject_test.go): a Pod that's already fully
// injected must get zero patch ops and must not increment
// WebhookInjectionsTotal, even though ShouldInject is true and the cache
// is synced.
func TestServeHTTP_AlreadyInjectedPod_NoPatchNoInjectionMetric(t *testing.T) {
	h, m := newTestHandlerWithMetrics(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "my-pod",
			Annotations: map[string]string{InjectAnnotation: "true"},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: volumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			InitContainers: []corev1.Container{
				{Name: initContainerName},
				{Name: sidecarContainerName},
			},
			Containers: []corev1.Container{
				{Name: "app", VolumeMounts: []corev1.VolumeMount{{Name: volumeName, MountPath: mountPath}}},
			},
		},
	}
	body := admissionReviewBody(t, "already-injected", "ns1", pod)

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
		t.Errorf("expected no patch for an already-injected Pod, got %s", review.Response.Patch)
	}
	if got := testutil.ToFloat64(m.WebhookInjectionsTotal); got != 0 {
		t.Errorf("WebhookInjectionsTotal: got %v, want 0 (nothing was actually injected)", got)
	}
	if got := testutil.ToFloat64(m.WebhookRequestsTotal.WithLabelValues("allowed")); got != 1 {
		t.Errorf("allowed count: got %v, want 1", got)
	}
}

func TestResolvedValues_NamespaceNotFound_ReturnsEmptyMapNotNil(t *testing.T) {
	cfg := &config.Config{}
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	svc.Cache.SetSynced()

	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	got, err := resolvedValues(req, svc, "nonexistent")
	if err != nil {
		t.Fatalf("unconfigured namespace should not be an error: %v", err)
	}
	if got == nil {
		t.Fatal("expected an empty, non-nil map for an unconfigured namespace, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected an empty map, got %v", got)
	}
}

// TestResolvedValues_StaleEntry_ReturnsError exercises the "genuine resolve
// error" branch (as opposed to the normal/expected ErrNamespaceNotFound case
// above) - a stale cache entry, gated by CacheEntryTTL, is the only realistic
// way to reach it here: ServeHTTP already gates on Cache.IsSynced() before
// calling resolvedValues, so ErrCacheNotSynced can't occur at this call site.
// The error must surface rather than becoming an empty map, since the caller
// denies admission on it.
func TestResolvedValues_StaleEntry_ReturnsError(t *testing.T) {
	cfg := &config.Config{CacheEntryTTL: time.Millisecond}
	svc := app.NewDiscoveryService(cfg, zap.NewNop(), nil)
	svc.Cache.Set(&app.ConfigEntry{
		Namespace: "acme",
		Sources:   map[string]map[string]any{"ConfigMap/x": {"k": "v"}},
		UpdatedAt: time.Now().Add(-time.Hour),
	})
	svc.Cache.SetSynced()

	req := httptest.NewRequest(http.MethodPost, "/mutate", nil)
	got, err := resolvedValues(req, svc, "acme")
	if err == nil {
		t.Fatalf("expected an error for a stale cache entry, got values %v", got)
	}
	if !errors.Is(err, app.ErrCacheEntryStale) {
		t.Errorf("error: got %v, want ErrCacheEntryStale", err)
	}
}

// brokenWriter always fails Write, simulating a client that disconnects
// mid-response - the only realistic way json.Encoder.Encode can fail here,
// since the AdmissionReview/AdmissionResponse types themselves always
// marshal successfully.
type brokenWriter struct{ http.ResponseWriter }

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write: broken pipe") }

func TestWriteAdmissionReview_EncodeErrorPropagates(t *testing.T) {
	w := brokenWriter{httptest.NewRecorder()}
	review := &admissionv1.AdmissionReview{}
	resp := &admissionv1.AdmissionResponse{Allowed: true}

	if err := writeAdmissionReview(w, review, resp); err == nil {
		t.Fatal("expected an error from a broken ResponseWriter, got nil")
	}
}
