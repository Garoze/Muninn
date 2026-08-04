package webhook

import (
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

// opsByPath indexes ops by Path for convenient assertions, since BuildPath's
// exact op ordering is an implementation detail, not part of its contract.
func opsByPath(ops []patchOperation) map[string]patchOperation {
	m := make(map[string]patchOperation, len(ops))
	for _, op := range ops {
		m[op.Path] = op
	}
	return m
}

func TestBuildPath_FreshPod_AddsVolumeInitSidecarAndAppMount(t *testing.T) {
	pod := freshPod()
	ops := BuildPath(pod, "acme", testConfig())

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

	sidecarOp, ok := byPath["/spec/containers/-"]
	if !ok {
		t.Fatal("missing /spec/containers/- add op (sidecar)")
	}
	sidecar, ok := sidecarOp.Value.(corev1.Container)
	if !ok || sidecar.Name != sidecarContainerName {
		t.Fatalf("sidecar op value: got %+v", sidecarOp.Value)
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

func TestBuildPath_ExistingVolumesAndInitContainers_Appends(t *testing.T) {
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

	ops := BuildPath(pod, "acme", testConfig())
	byPath := opsByPath(ops)

	if _, ok := byPath["/spec/volumes/-"]; !ok {
		t.Errorf("expected append-style /spec/volumes/- op when volumes already exist, got %+v", ops)
	}
	if _, ok := byPath["/spec/initContainers/-"]; !ok {
		t.Errorf("expected append-style /spec/initContainers/- op when initContainers already exist, got %+v", ops)
	}
	if _, ok := byPath["/spec/containers/0/volumeMounts/-"]; !ok {
		t.Errorf("expected append-style /spec/containers/0/volumeMounts/- op when the container already has mounts, got %+v", ops)
	}
	if _, ok := byPath["/spec/containers/-"]; !ok {
		t.Errorf("expected sidecar to still be appended, got %+v", ops)
	}
}

func TestBuildPath_AlreadyInjected_IsIdempotent(t *testing.T) {
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

	ops := BuildPath(pod, "acme", testConfig())
	if len(ops) != 0 {
		t.Errorf("expected no ops for an already-fully-injected Pod (idempotency), got %+v", ops)
	}
}

func TestAddAppVolumeMountOps_MultipleContainers_MixedState(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "already-mounted", VolumeMounts: []corev1.VolumeMount{{Name: volumeName, MountPath: mountPath}}},
				{Name: "no-mounts-yet"},
				{Name: "has-other-mount", VolumeMounts: []corev1.VolumeMount{{Name: "unrelated", MountPath: "/x"}}},
			},
		},
	}

	ops := addAppVolumeMountOps(pod)
	byPath := opsByPath(ops)

	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2 (skip the already-mounted container): %+v", len(ops), ops)
	}
	if _, ok := byPath["/spec/containers/0/volumeMounts"]; ok {
		t.Error("container 0 already has the mount, should have been skipped")
	}
	if _, ok := byPath["/spec/containers/1/volumeMounts"]; !ok {
		t.Error("container 1 has no volumeMounts yet, expected the whole-array add form")
	}
	if _, ok := byPath["/spec/containers/2/volumeMounts/-"]; !ok {
		t.Error("container 2 has an unrelated mount, expected the append form")
	}
}

func TestHasVolumeMount(t *testing.T) {
	c := corev1.Container{VolumeMounts: []corev1.VolumeMount{{Name: "a"}, {Name: "b"}}}
	if !hasVolumeMount(c, "a") {
		t.Error("expected true for a mount that exists")
	}
	if hasVolumeMount(c, "c") {
		t.Error("expected false for a mount that doesn't exist")
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
