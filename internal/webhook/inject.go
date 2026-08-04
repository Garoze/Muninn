package webhook

import (
	"fmt"
	"path/filepath"

	"github.com/garoze/muninn/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

const (
	// InjectAnnotation opts a Pod into config injection. Absent ot not
	// "true" means the webhook leaves the Pod untouched.
	InjectAnnotation = "muninn.io/inject"

	volumeName           = "muninn-config"
	mountPath            = "/etc/muninn"
	configFileName       = "config.yaml"
	initContainerName    = "muninn-resolve-init"
	sidecarContainerName = "muninn-resolve-sidecar"
	sidecarWatchInterval = "15s"
)

// patchOperation is a single RFC 6902 JSON Patch operation.
type patchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// ShouldInject reports whether pod opted in via InjectAnnotation
func ShouldInject(pod *corev1.Pod) bool {
	return pod.GetAnnotations()[InjectAnnotation] == "true"
}

// BuildPath return the JSON Path operations needed to inject th shared
// volume, init container, and sidecar into pod, and to mount that volume
// into every container the Pod already had - or nil if they're already
// present. namespace somes from the AdmissionRequest, not pod.Namespace:
// the request's namespace field is uthoritative at admission time,
// independent of whether the submitted object's own metadata.namespace was
// populated by the client.
//
// The existing-container mount is what makes this a zero-client-code
// integration: without it, a consumer would still need to know Muninn's
// internal volume name/mount path to read the resolved config themselves.
//
// Idempotent via equality.Semantic.DeepEqual: each relevant Pod spec field
// (volumes, initContainers, containers, each container's volumeMounts) is
// compared as a whole against the desired value for that field, and a patch
// op is only emitted when they differ. A webhook re-invoked for the same
// admission request (the API server can do this) sees every field already
// equal to its desired value and produces zero ops, not a duplicate
// volume/container/mount - non-idempotent mutation webhooks cause infinite
// reconciliation loops against the API server.
func BuildPath(pod *corev1.Pod, namespace string, cfg *config.Config) []patchOperation {
	var ops []patchOperation

	if op := volumesOp(pod.Spec.Volumes); op != nil {
		ops = append(ops, *op)
	}

	if op := initContainersOp(pod.Spec.InitContainers, namespace, cfg); op != nil {
		ops = append(ops, *op)
	}

	if op := containersOp(pod.Spec.Containers, namespace, cfg); op != nil {
		ops = append(ops, *op)
	}

	ops = append(ops, appVolumeMountOps(pod.Spec.Containers)...)

	return ops
}

// volumesOp compares the desired /spec/volumes value (current plus the
// shared muninn-config volume, unless a volume by that name already exists)
// against the current value via equality.Semantic.DeepEqual, returning a
// patch op only when they differ.
func volumesOp(current []corev1.Volume) *patchOperation {
	vol := corev1.Volume{
		Name:         volumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}

	desired := withVolume(current, vol)
	if equality.Semantic.DeepEqual(desired, current) {
		return nil
	}

	// JSON Patch's "add" to an array path requires the whole array as the
	// value when the array doesn't exist yet (nil/empty on a fresh Pod
	// spec, since corev1.PodSpec.Volumes is omitempty); once it exists,
	// "replace" is the correct op for a whole-array value.
	if len(current) == 0 {
		return &patchOperation{Op: "add", Path: "/spec/volumes", Value: desired}
	}
	return &patchOperation{Op: "replace", Path: "/spec/volumes", Value: desired}
}

// initContainersOp mirrors volumesOp for /spec/initContainers, which is
// also omitempty and nil on most Pods.
func initContainersOp(current []corev1.Container, namespace string, cfg *config.Config) *patchOperation {
	c := buildResolveContainer(initContainerName, namespace, cfg, false)

	desired := withContainer(current, c)
	if equality.Semantic.DeepEqual(desired, current) {
		return nil
	}

	if len(current) == 0 {
		return &patchOperation{Op: "add", Path: "/spec/initContainers", Value: desired}
	}
	return &patchOperation{Op: "replace", Path: "/spec/initContainers", Value: desired}
}

// containersOp mirrors volumesOp for /spec/containers, always via
// "replace": a valid Pod always has at least one container already, so
// /spec/containers is never empty/missing at admission time.
func containersOp(current []corev1.Container, namespace string, cfg *config.Config) *patchOperation {
	c := buildResolveContainer(sidecarContainerName, namespace, cfg, true)

	desired := withContainer(current, c)
	if equality.Semantic.DeepEqual(desired, current) {
		return nil
	}
	return &patchOperation{Op: "replace", Path: "/spec/containers", Value: desired}
}

// appVolumeMountOps mounts the shared volume into every container the Pod
// already had at admission time (not the sidecar being injected in this
// same call - that one mounts itself in buildResolveContainer), so the
// application itself can read the resolved config file without any change
// to its own manifest beyond the opt-in annotation. containers is
// pod.Spec.Containers as decoded from the admission request, so indices
// here stay valid even after containersOp's /spec/containers replace,
// which only appends the sidecar and leaves existing entries' positions
// unchanged.
func appVolumeMountOps(containers []corev1.Container) []patchOperation {
	var ops []patchOperation

	mount := corev1.VolumeMount{Name: volumeName, MountPath: mountPath}

	for i, c := range containers {
		desired := withVolumeMount(c.VolumeMounts, mount)
		if equality.Semantic.DeepEqual(desired, c.VolumeMounts) {
			continue
		}

		if len(c.VolumeMounts) == 0 {
			ops = append(ops, patchOperation{
				Op:    "add",
				Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts", i),
				Value: desired,
			})
			continue
		}

		ops = append(ops, patchOperation{
			Op:    "replace",
			Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts", i),
			Value: desired,
		})
	}

	return ops
}

// withVolume returns vols with vol appended, unless a volume already named
// vol.Name is present - matched by name, not full equality, so this never
// introduces a second volume sharing a name the real Kubernetes API would
// reject as invalid, even if that pre-existing volume's contents differ
// from what Muninn would have injected.
func withVolume(vols []corev1.Volume, vol corev1.Volume) []corev1.Volume {
	for _, v := range vols {
		if v.Name == vol.Name {
			return vols
		}
	}

	out := make([]corev1.Volume, len(vols), len(vols)+1)
	copy(out, vols)
	return append(out, vol)
}

// withContainer is withVolume's counterpart for a container slice, matched
// by container Name for the same duplicate-name reason.
func withContainer(containers []corev1.Container, c corev1.Container) []corev1.Container {
	for _, existing := range containers {
		if existing.Name == c.Name {
			return containers
		}
	}

	out := make([]corev1.Container, len(containers), len(containers)+1)
	copy(out, containers)
	return append(out, c)
}

// withVolumeMount is withVolume's counterpart for a single container's
// VolumeMounts, matched by mount Name for the same duplicate-name reason.
func withVolumeMount(mounts []corev1.VolumeMount, m corev1.VolumeMount) []corev1.VolumeMount {
	for _, existing := range mounts {
		if existing.Name == m.Name {
			return mounts
		}
	}

	out := make([]corev1.VolumeMount, len(mounts), len(mounts)+1)
	copy(out, mounts)
	return append(out, m)
}

// buildResolveContainer builds the init container (watch=false, runs once
// and exists) or sidecar (watch=true, poll and rewrites on drif) - both
// invoke the same muninn binary in `resolve` mode, just with different
// arguments.
func buildResolveContainer(name, namespace string, cfg *config.Config, watch bool) corev1.Container {
	args := []string{
		"resolve",
		"--addr", cfg.SelfAddr,
		"--namespace", namespace,
		"--out", filepath.Join(mountPath, configFileName),
	}

	if watch {
		args = append(args, "--watch", "--interval", sidecarWatchInterval)
	}

	return corev1.Container{
		Name: name,
		// Explicit, not left to default: a ":latest" tag (InjectImage's
		// usual local-dev value) defaults to PullAlways otherwise, which
		// tries to pull from a registry named "localhost" and fails -
		// IfNotPresent uses an already-loaded local image but still pulls
		// in a real registry-backed deployment, unlike PullNever.
		Image:           cfg.InjectImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            args,
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: mountPath},
		},
	}
}
