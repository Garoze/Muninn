package webhook

import (
	"fmt"
	"path/filepath"

	"github.com/garoze/muninn/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

const (
	// InjectAnnotation opts a Pod into config injection. Absent or not
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

// BuildPath returns the JSON Patch operations needed to inject the shared
// volume, init container, and sidecar into pod, and to mount that volume
// into every existing container - or nil if already present. namespace comes
// from the AdmissionRequest rather than pod.Namespace, since the latter may
// be unpopulated by the client at admission time.
//
// Idempotent via equality.Semantic.DeepEqual against each relevant Pod spec
// field: a webhook re-invoked for the same admission request produces zero
// ops instead of duplicating a volume/container/mount.
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

func volumesOp(current []corev1.Volume) *patchOperation {
	vol := corev1.Volume{
		Name:         volumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}

	desired := withVolume(current, vol)
	if equality.Semantic.DeepEqual(desired, current) {
		return nil
	}

	// RFC 6902 "add" requires the whole array when the path doesn't exist
	// yet; "replace" once it does.
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
// already had, so the application can read the resolved config file with no
// manifest change beyond the opt-in annotation. Indices index into the
// original containers slice; containersOp only appends the sidecar, so
// existing positions stay valid.
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

// withVolume appends vol unless a volume by that name already exists -
// matched by name since Kubernetes rejects duplicate volume names.
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

// buildResolveContainer builds the init container (watch=false, runs once)
// or sidecar (watch=true, polls and rewrites on drift).
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
		// Explicit: a ":latest" tag defaults to PullAlways, which fails
		// against a locally-loaded, unpushed image.
		Image:           cfg.InjectImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            args,
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: mountPath},
		},
	}
}
