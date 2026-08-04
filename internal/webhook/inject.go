package webhook

import (
	"fmt"
	"path/filepath"

	"github.com/garoze/muninn/internal/config"
	corev1 "k8s.io/api/core/v1"
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
// Idempotent by construction: each piece is checked for existence by name
// before being added, so a webhook re-invoked for the same admission
// request (the API server can do this) produces the same result, not a
// duplicate volume/container/mount.
func BuildPath(pod *corev1.Pod, namespace string, cfg *config.Config) []patchOperation {
	var ops []patchOperation

	if !hasVolume(pod, volumeName) {
		ops = append(ops, addVolumeOp(pod, corev1.Volume{
			Name:         volumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		}))
	}

	if !hasContainer(pod.Spec.InitContainers, initContainerName) {
		ops = append(ops, addInitContainerOp(pod, buildResolveContainer(
			initContainerName, namespace, cfg, false,
		)))
	}

	if !hasContainer(pod.Spec.Containers, sidecarContainerName) {
		ops = append(ops, addContainerOp(buildResolveContainer(
			sidecarContainerName, namespace, cfg, true,
		)))
	}

	ops = append(ops, addAppVolumeMountOps(pod)...)

	return ops
}

// addAppVolumeMountOps mounts the shared volume into every container the
// Pod already had at admission time (not the sidecar being injected in
// this same call - that one mounts itself in buildResolveContainer), so
// the application itself can read the resolved config file without any
// change to its own manifest beyond the opt-in annotation.
func addAppVolumeMountOps(pod *corev1.Pod) []patchOperation {
	var ops []patchOperation

	mount := corev1.VolumeMount{Name: volumeName, MountPath: mountPath}

	for i, c := range pod.Spec.Containers {
		if hasVolumeMount(c, volumeName) {
			continue
		}

		if len(c.VolumeMounts) == 0 {
			ops = append(ops, patchOperation{
				Op:    "add",
				Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts", i),
				Value: []corev1.VolumeMount{mount},
			})
			continue
		}

		ops = append(ops, patchOperation{
			Op:    "add",
			Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts/-", i),
			Value: mount,
		})
	}

	return ops
}

func hasVolumeMount(c corev1.Container, name string) bool {
	for _, vm := range c.VolumeMounts {
		if vm.Name == name {
			return true
		}
	}

	return false
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

func hasVolume(pod *corev1.Pod, name string) bool {
	for _, v := range pod.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}

	return false
}

func hasContainer(containers []corev1.Container, name string) bool {
	for _, c := range containers {
		if c.Name == name {
			return true
		}
	}

	return false
}

// addVolumeOp adds vol to /spec/volumes. JSON Patch's "add" to an array
// path requires the whole array as the value when the array doesn't exist
// yet (nil/empty on a fresh Pod spec); once it exists, "/spec/volumes/-"
// appends a single element instead.
func addVolumeOp(pod *corev1.Pod, vol corev1.Volume) patchOperation {
	if len(pod.Spec.Volumes) == 0 {
		return patchOperation{Op: "add", Path: "/spec/volumes", Value: []corev1.Volume{vol}}
	}

	return patchOperation{Op: "add", Path: "/spec/volumes/-", Value: vol}
}

// addInitContainerOp mirros addVolumeOp's array-vs-append logic for
// /spec/initContainers, which is nil on most Pods (init containers are
// less common than volumes on a bare Pod spec).
func addInitContainerOp(pod *corev1.Pod, c corev1.Container) patchOperation {
	if len(pod.Spec.InitContainers) == 0 {
		return patchOperation{Op: "add", Path: "/spec/initContainers", Value: []corev1.Container{c}}
	}

	return patchOperation{Op: "add", Path: "/spec/initContainers/-", Value: c}
}

// addContainerOp always appends: a valid Pod always had at least one
// container already, so /spec/contrainers is never empty at admission time.
func addContainerOp(c corev1.Container) patchOperation {
	return patchOperation{Op: "add", Path: "/spec/containers/-", Value: c}
}
