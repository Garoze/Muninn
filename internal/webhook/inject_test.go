package webhook

import (
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/garoze/muninn/internal/config"
)

func TestShouldInject(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{"no annotations", nil, false},
		{"annotation absent", map[string]string{"other": "true"}, false},
		{"annotation false", map[string]string{InjectAnnotation: "false"}, false},
		{"annotation not exactly true", map[string]string{InjectAnnotation: "yes"}, false},
		{"annotation true", map[string]string{InjectAnnotation: "true"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			if got := ShouldInject(pod); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func testConfig() *config.Config {
	return &config.Config{
		SelfAddr:    "muninn.muninn-system.svc.cluster.local:5010",
		InjectImage: "localhost/muninn:latest",
	}
}

func freshPod() *corev1.Pod {
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "example/app:latest"},
			},
		},
	}
}

// opsByPath indexes ops by Path for convenient assertions, since BuildPatch's
// exact op ordering is an implementation detail, not part of its contract.
func opsByPath(ops []patchOperation) map[string]patchOperation {
	m := make(map[string]patchOperation, len(ops))
	for _, op := range ops {
		m[op.Path] = op
	}
	return m
}

func TestBuildPatch_FreshPod_AddsVolumeInitSidecarAndAppMount(t *testing.T) {
	pod := freshPod()
	ops := BuildPatch(pod, "acme", testConfig(), nil)

	if len(ops) != 4 {
		t.Fatalf("got %d ops, want 4: %+v", len(ops), ops)
	}
	byPath := opsByPath(ops)

	volOp, ok := byPath["/spec/volumes"]
	if !ok {
		t.Fatal("missing /spec/volumes add op")
	}
	vols, ok := volOp.Value.([]corev1.Volume)
	if !ok || len(vols) != 1 || vols[0].Name != volumeName || vols[0].EmptyDir == nil {
		t.Errorf("volumes op value: got %+v", volOp.Value)
	}

	initOp, ok := byPath["/spec/initContainers"]
	if !ok {
		t.Fatal("missing /spec/initContainers add op")
	}
	initContainers, ok := initOp.Value.([]corev1.Container)
	if !ok || len(initContainers) != 1 {
		t.Fatalf("initContainers op value: got %+v", initOp.Value)
	}
	initC := initContainers[0]
	if initC.Name != initContainerName {
		t.Errorf("init container name: got %q, want %q", initC.Name, initContainerName)
	}
	if initC.Image != "localhost/muninn:latest" {
		t.Errorf("init container image: got %q", initC.Image)
	}
	if initC.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("init container ImagePullPolicy: got %q, want %q", initC.ImagePullPolicy, corev1.PullIfNotPresent)
	}
	wantInitArgs := []string{
		"resolve", "--addr", "muninn.muninn-system.svc.cluster.local:5010",
		"--namespace", "acme", "--out", "/etc/muninn/config.yaml",
	}
	if !stringSlicesEqual(initC.Args, wantInitArgs) {
		t.Errorf("init container args: got %v, want %v", initC.Args, wantInitArgs)
	}

	containersOp, ok := byPath["/spec/containers"]
	if !ok || containersOp.Op != "replace" {
		t.Fatal("missing /spec/containers replace op (sidecar)")
	}
	containers, ok := containersOp.Value.([]corev1.Container)
	if !ok || len(containers) != 2 || containers[0].Name != "app" {
		t.Fatalf("containers op value: got %+v", containersOp.Value)
	}
	sidecar := containers[1]
	if sidecar.Name != sidecarContainerName {
		t.Fatalf("sidecar name: got %q, want %q", sidecar.Name, sidecarContainerName)
	}
	wantSidecarArgs := append(append([]string{}, wantInitArgs...), "--watch", "--interval", sidecarWatchInterval)
	if !stringSlicesEqual(sidecar.Args, wantSidecarArgs) {
		t.Errorf("sidecar args: got %v, want %v", sidecar.Args, wantSidecarArgs)
	}

	mountOp, ok := byPath["/spec/containers/0/volumeMounts"]
	if !ok {
		t.Fatal("missing /spec/containers/0/volumeMounts add op (app container mount)")
	}
	mounts, ok := mountOp.Value.([]corev1.VolumeMount)
	if !ok || len(mounts) != 1 || mounts[0].Name != volumeName || mounts[0].MountPath != mountPath {
		t.Errorf("app container volumeMounts op value: got %+v", mountOp.Value)
	}
}

// TestBuildPatch_WithSecretRefs_AddsCSIVolumeAndMount is a regression test
// for the CSI VolumeMount.Name having to match csiSecretVolume's Volume.Name
// (muninn-secrets) rather than the CSI driver's own name
// (secrets-store.csi.x-k8s.io) - a mismatch there passes admission but the
// kubelet then rejects the Pod outright at mount time, since a VolumeMount
// references a volume in the same Pod spec, not the driver serving it.
func TestBuildPatch_WithSecretRefs_AddsCSIVolumeAndMount(t *testing.T) {
	pod := freshPod()
	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password"}}

	ops := BuildPatch(pod, "acme", testConfig(), refs)
	byPath := opsByPath(ops)

	volOp, ok := byPath["/spec/volumes"]
	if !ok {
		t.Fatal("missing /spec/volumes add op")
	}
	vols, ok := volOp.Value.([]corev1.Volume)
	if !ok || len(vols) != 2 {
		t.Fatalf("volumes op value: got %+v, want config volume + CSI secrets volume", volOp.Value)
	}
	csiVol := vols[1]
	if csiVol.Name != csiVolumeName {
		t.Errorf("CSI volume name: got %q, want %q", csiVol.Name, csiVolumeName)
	}
	if csiVol.CSI == nil || csiVol.CSI.Driver != csiDriverName {
		t.Fatalf("CSI volume source: got %+v", csiVol.VolumeSource)
	}
	if csiVol.CSI.ReadOnly == nil || !*csiVol.CSI.ReadOnly {
		t.Error("expected CSI volume to be ReadOnly")
	}
	wantSPC := "muninn-secrets-acme"
	if got := csiVol.CSI.VolumeAttributes["secretProviderClass"]; got != wantSPC {
		t.Errorf("secretProviderClass attribute: got %q, want %q", got, wantSPC)
	}

	mountOp, ok := byPath["/spec/containers/0/volumeMounts"]
	if !ok {
		t.Fatal("missing /spec/containers/0/volumeMounts add op")
	}
	mounts, ok := mountOp.Value.([]corev1.VolumeMount)
	if !ok || len(mounts) != 2 {
		t.Fatalf("app container volumeMounts op value: got %+v, want config mount + CSI mount", mountOp.Value)
	}
	csiMount := mounts[1]
	// The mount must reference the Volume.Name added above (csiVolumeName),
	// not the CSI driver's own name - that was the actual bug.
	if csiMount.Name != csiVolumeName {
		t.Errorf("CSI volume mount Name: got %q, want %q (must match the Volume.Name, not the driver name)", csiMount.Name, csiVolumeName)
	}
	if csiMount.MountPath != csiMountPath {
		t.Errorf("CSI volume mount path: got %q, want %q", csiMount.MountPath, csiMountPath)
	}
	if !csiMount.ReadOnly {
		t.Error("expected CSI volume mount to be ReadOnly")
	}
}

func TestBuildPatch_NoSecretRefs_NoCSIVolumeOrMount(t *testing.T) {
	pod := freshPod()
	ops := BuildPatch(pod, "acme", testConfig(), nil)
	byPath := opsByPath(ops)

	vols := byPath["/spec/volumes"].Value.([]corev1.Volume)
	if len(vols) != 1 {
		t.Errorf("expected only the config volume with no refs, got %+v", vols)
	}

	mounts := byPath["/spec/containers/0/volumeMounts"].Value.([]corev1.VolumeMount)
	if len(mounts) != 1 {
		t.Errorf("expected only the config mount with no refs, got %+v", mounts)
	}
}

func TestBuildPatch_ExistingVolumesAndInitContainers_ReplacesWholeField(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "other-volume"}},
			InitContainers: []corev1.Container{
				{Name: "other-init"},
			},
			Containers: []corev1.Container{
				{
					Name:         "app",
					VolumeMounts: []corev1.VolumeMount{{Name: "other-volume", MountPath: "/other"}},
				},
			},
		},
	}

	ops := BuildPatch(pod, "acme", testConfig(), nil)
	byPath := opsByPath(ops)

	volOp, ok := byPath["/spec/volumes"]
	if !ok || volOp.Op != "replace" {
		t.Fatalf("expected a replace op for /spec/volumes when volumes already exist, got %+v", ops)
	}
	vols := volOp.Value.([]corev1.Volume)
	if len(vols) != 2 || vols[0].Name != "other-volume" || vols[1].Name != volumeName {
		t.Errorf("expected existing volume preserved plus muninn-config appended, got %+v", vols)
	}

	initOp, ok := byPath["/spec/initContainers"]
	if !ok || initOp.Op != "replace" {
		t.Fatalf("expected a replace op for /spec/initContainers when initContainers already exist, got %+v", ops)
	}
	initContainers := initOp.Value.([]corev1.Container)
	if len(initContainers) != 2 || initContainers[0].Name != "other-init" || initContainers[1].Name != initContainerName {
		t.Errorf("expected existing init container preserved plus muninn's appended, got %+v", initContainers)
	}

	mountOp, ok := byPath["/spec/containers/0/volumeMounts"]
	if !ok || mountOp.Op != "replace" {
		t.Fatalf("expected a replace op for the app container's volumeMounts, got %+v", ops)
	}
	mounts := mountOp.Value.([]corev1.VolumeMount)
	if len(mounts) != 2 || mounts[0].Name != "other-volume" || mounts[1].Name != volumeName {
		t.Errorf("expected existing mount preserved plus muninn-config appended, got %+v", mounts)
	}

	containersOp, ok := byPath["/spec/containers"]
	if !ok || containersOp.Op != "replace" {
		t.Fatalf("expected a replace op for /spec/containers (sidecar addition), got %+v", ops)
	}
	containers := containersOp.Value.([]corev1.Container)
	if len(containers) != 2 || containers[0].Name != "app" || containers[1].Name != sidecarContainerName {
		t.Errorf("expected existing app container preserved plus sidecar appended, got %+v", containers)
	}
}

func TestBuildPatch_AlreadyInjected_IsIdempotent(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: volumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			InitContainers: []corev1.Container{
				{Name: initContainerName},
			},
			Containers: []corev1.Container{
				{
					Name:         "app",
					VolumeMounts: []corev1.VolumeMount{{Name: volumeName, MountPath: mountPath}},
				},
				{
					Name:         sidecarContainerName,
					VolumeMounts: []corev1.VolumeMount{{Name: volumeName, MountPath: mountPath}},
				},
			},
		},
	}

	ops := BuildPatch(pod, "acme", testConfig(), nil)
	if len(ops) != 0 {
		t.Errorf("expected no ops for an already-fully-injected Pod (idempotency), got %+v", ops)
	}
}

// TestBuildPatch_AlreadyInjected_WithRefs_IsIdempotent is the refs-bearing
// counterpart to TestBuildPatch_AlreadyInjected_IsIdempotent. The injected
// sidecar is in Spec.Containers by the time a re-invoked admission runs, so
// mounting the CSI secrets volume into every container would emit an op for it
// and put consumer secrets inside a Muninn-authored container.
func TestBuildPatch_AlreadyInjected_WithRefs_IsIdempotent(t *testing.T) {
	cfg := testConfig()
	refs := []SecretRef{{Key: "db_password_ref", ObjectName: "db_password", Provider: "vault", Path: "secret/data/prod/db-password"}}

	// Build the Pod the way a first admission actually leaves it, rather than
	// hand-writing the expected shape, so this can't drift from BuildPatch.
	fresh := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
	}
	injected := applyOps(t, fresh, BuildPatch(fresh, "acme", cfg, refs))

	ops := BuildPatch(injected, "acme", cfg, refs)
	if len(ops) != 0 {
		t.Errorf("expected no ops re-invoking on an already-injected Pod with refs, got %+v", ops)
	}

	for _, c := range injected.Spec.Containers {
		if c.Name != sidecarContainerName {
			continue
		}
		for _, m := range c.VolumeMounts {
			if m.Name == csiVolumeName {
				t.Errorf("sidecar must not mount the consumer's secrets volume, got %+v", c.VolumeMounts)
			}
		}
	}
}

// applyOps applies the subset of JSON Patch ops BuildPatch emits (add/replace on
// whole spec fields) so a test can work from the Pod a real admission produces.
func applyOps(t *testing.T, pod *corev1.Pod, ops []patchOperation) *corev1.Pod {
	t.Helper()

	out := pod.DeepCopy()
	for _, op := range ops {
		switch {
		case op.Path == "/spec/volumes":
			out.Spec.Volumes = op.Value.([]corev1.Volume)
		case op.Path == "/spec/initContainers":
			out.Spec.InitContainers = op.Value.([]corev1.Container)
		case op.Path == "/spec/containers":
			out.Spec.Containers = op.Value.([]corev1.Container)
		case strings.HasPrefix(op.Path, "/spec/containers/") && strings.HasSuffix(op.Path, "/volumeMounts"):
			idx, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(op.Path, "/spec/containers/"), "/volumeMounts"))
			if err != nil {
				t.Fatalf("unexpected container index in path %q: %v", op.Path, err)
			}
			out.Spec.Containers[idx].VolumeMounts = op.Value.([]corev1.VolumeMount)
		default:
			t.Fatalf("applyOps does not handle path %q", op.Path)
		}
	}
	return out
}

// The webhook adds these containers to a Pod its owner did not write, so they
// have to satisfy the namespace's own admission constraints unaided: a
// ResourceQuota rejects containers without requests and limits, and the
// restricted Pod Security standard rejects an unset securityContext.
func TestBuildResolveContainer_SetsResourcesAndSecurityContext(t *testing.T) {
	for _, watch := range []bool{false, true} {
		c := buildResolveContainer("muninn-test", "acme", testConfig(), watch)

		if c.Resources.Requests.Cpu().IsZero() || c.Resources.Requests.Memory().IsZero() {
			t.Errorf("watch=%v: missing resource requests: %+v", watch, c.Resources)
		}
		if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
			t.Errorf("watch=%v: missing resource limits: %+v", watch, c.Resources)
		}

		sc := c.SecurityContext
		if sc == nil {
			t.Fatalf("watch=%v: missing securityContext", watch)
		}
		if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
			t.Errorf("watch=%v: allowPrivilegeEscalation must be false", watch)
		}
		if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
			t.Errorf("watch=%v: runAsNonRoot must be true", watch)
		}
		if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
			t.Errorf("watch=%v: readOnlyRootFilesystem must be true", watch)
		}
		if sc.Capabilities == nil || len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" {
			t.Errorf("watch=%v: capabilities must drop ALL, got %+v", watch, sc.Capabilities)
		}
	}
}

func TestAppVolumeMountOps_MultipleContainers_MixedState(t *testing.T) {
	containers := []corev1.Container{
		{Name: "already-mounted", VolumeMounts: []corev1.VolumeMount{{Name: volumeName, MountPath: mountPath}}},
		{Name: "no-mounts-yet"},
		{Name: "has-other-mount", VolumeMounts: []corev1.VolumeMount{{Name: "unrelated", MountPath: "/x"}}},
	}

	ops := appVolumeMountOps(containers, nil)
	byPath := opsByPath(ops)

	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2 (skip the already-mounted container): %+v", len(ops), ops)
	}
	if _, ok := byPath["/spec/containers/0/volumeMounts"]; ok {
		t.Error("container 0 already has the mount, should have been skipped (DeepEqual already-equal)")
	}
	addOp, ok := byPath["/spec/containers/1/volumeMounts"]
	if !ok || addOp.Op != "add" {
		t.Error("container 1 has no volumeMounts yet, expected the whole-array add form")
	}
	replaceOp, ok := byPath["/spec/containers/2/volumeMounts"]
	if !ok || replaceOp.Op != "replace" {
		t.Error("container 2 has an unrelated mount, expected the whole-array replace form")
	}
}

func TestWithVolumeMount(t *testing.T) {
	mounts := []corev1.VolumeMount{{Name: "a"}, {Name: "b"}}

	if got := withVolumeMount(mounts, corev1.VolumeMount{Name: "a", MountPath: "/different"}); len(got) != 2 {
		t.Errorf("expected no change for an already-present name, got %+v", got)
	}

	got := withVolumeMount(mounts, corev1.VolumeMount{Name: "c"})
	if len(got) != 3 || got[2].Name != "c" {
		t.Errorf("expected mount appended for a new name, got %+v", got)
	}
}

func TestWithVolume(t *testing.T) {
	vols := []corev1.Volume{{Name: "a"}, {Name: "b"}}

	if got := withVolume(vols, corev1.Volume{Name: "a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}); len(got) != 2 {
		t.Errorf("expected no change for an already-present name, got %+v", got)
	}

	got := withVolume(vols, corev1.Volume{Name: "c"})
	if len(got) != 3 || got[2].Name != "c" {
		t.Errorf("expected volume appended for a new name, got %+v", got)
	}
}

func TestWithContainer(t *testing.T) {
	containers := []corev1.Container{{Name: "a"}, {Name: "b"}}

	if got := withContainer(containers, corev1.Container{Name: "a", Image: "different:latest"}); len(got) != 2 {
		t.Errorf("expected no change for an already-present name, got %+v", got)
	}

	got := withContainer(containers, corev1.Container{Name: "c"})
	if len(got) != 3 || got[2].Name != "c" {
		t.Errorf("expected container appended for a new name, got %+v", got)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
