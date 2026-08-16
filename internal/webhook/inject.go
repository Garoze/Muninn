package webhook

import (
	"fmt"
	"path/filepath"

	"github.com/garoze/muninn/internal/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
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

	csiVolumeName = "muninn-secrets"
	csiMountPath  = "/mnt/secrets-store"
	csiDriverName = "secrets-store.csi.k8s.io"
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

// BuildPatch returns the JSON Patch operations needed to inject the shared
// volume, init container, and sidecar into pod, and to mount those volumes
// into every existing container - or nil if already present. namespace comes
// from the AdmissionRequest rather than pod.Namespace, since the latter may
// be unpopulated by the client at admission time. refs being non-empty adds
// the CSI secrets volume/mount alongside the config ones; the caller is
// responsible for having already reconciled the SecretProviderClass refs
// point at.
//
// Idempotent via equality.Semantic.DeepEqual against each relevant Pod spec
// field: a webhook re-invoked for the same admission request produces zero
// ops instead of duplicating a volume/container/mount.
func BuildPatch(pod *corev1.Pod, namespace string, cfg *config.Config, refs []SecretRef) []patchOperation {
	var ops []patchOperation

	if op := volumesOp(pod.Spec.Volumes, namespace, refs); op != nil {
		ops = append(ops, *op)
	}

	if op := initContainersOp(pod.Spec.InitContainers, namespace, cfg); op != nil {
		ops = append(ops, *op)
	}

	ops = append(ops, appVolumeMountOps(pod.Spec.Containers, refs)...)

	return ops
}

func volumesOp(current []corev1.Volume, namespace string, refs []SecretRef) *patchOperation {
	vol := corev1.Volume{
		Name:         volumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}

	desired := withVolume(current, vol)
	if len(refs) > 0 {
		desired = withVolume(desired, csiSecretVolume(namespace))
	}

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

// csiSecretVolume references the namespace's SecretProviderClass - the
// kubelet performs the actual mount via the CSI driver, not this webhook.
func csiSecretVolume(namespace string) corev1.Volume {
	readOnly := true
	return corev1.Volume{
		Name: csiVolumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   csiDriverName,
				ReadOnly: &readOnly,
				VolumeAttributes: map[string]string{
					"secretProviderClass": spcName(namespace),
				},
			},
		},
	}
}

// initContainersOp mirrors volumesOp for /spec/initContainers, which is
// also omitempty and nil on most Pods.
//
// Both injected containers go here. The one-shot resolve runs to completion
// first, then the watching one - a native sidecar, an init container carrying
// restartPolicy: Always - starts and keeps running for the Pod's lifetime.
//
// The watching container must not be an ordinary entry in /spec/containers.
// A Pod's ordinary containers must all exit for the Pod to succeed, so a
// container that never exits keeps an annotated Job or CronJob Pod running
// forever. A native sidecar is excluded from that condition, which is what
// makes injection safe for a Pod that is meant to finish.
func initContainersOp(current []corev1.Container, namespace string, cfg *config.Config) *patchOperation {
	desired := withContainer(current, buildResolveContainer(initContainerName, namespace, cfg, false))
	desired = withContainer(desired, buildSidecarContainer(namespace, cfg))

	if equality.Semantic.DeepEqual(desired, current) {
		return nil
	}

	if len(current) == 0 {
		return &patchOperation{Op: "add", Path: "/spec/initContainers", Value: desired}
	}

	return &patchOperation{Op: "replace", Path: "/spec/initContainers", Value: desired}
}

// buildSidecarContainer is the watching resolve container as a native
// sidecar. restartPolicy: Always on an init container is what marks it as
// one: the Pod's startup proceeds once it has started rather than waiting for
// it to exit, and it is restarted for as long as the Pod runs.
func buildSidecarContainer(namespace string, cfg *config.Config) corev1.Container {
	c := buildResolveContainer(sidecarContainerName, namespace, cfg, true)
	always := corev1.ContainerRestartPolicyAlways
	c.RestartPolicy = &always
	return c
}

// appVolumeMountOps mounts the shared volume(s) into every container the Pod
// already had, so the application can read the resolved config file (and,
// when refs is non-empty, the CSI-mounted secrets) with no manifest change
// beyond the opt-in annotation. Indices index into the original containers
// slice; containersOp only appends the sidecar, so existing positions stay
// valid.
//
// Muninn's own injected containers are skipped. On a re-invoked admission the
// sidecar is already in this slice, and mounting the secrets volume into it
// would both break idempotency and hand a Muninn-authored container read
// access to the consumer's secrets, which the CSI delivery model exists to
// avoid (see docs/adr/0012-csi-secret-delivery.md).
func appVolumeMountOps(containers []corev1.Container, refs []SecretRef) []patchOperation {
	var ops []patchOperation

	mounts := []corev1.VolumeMount{{Name: volumeName, MountPath: mountPath}}
	if len(refs) > 0 {
		// Name must match csiSecretVolume's Volume.Name, not the CSI
		// driver's own name - a VolumeMount references a volume in this
		// Pod's spec, not the driver serving it.
		mounts = append(mounts, corev1.VolumeMount{Name: csiVolumeName, MountPath: csiMountPath, ReadOnly: true})
	}

	for i, c := range containers {
		if c.Name == sidecarContainerName || c.Name == initContainerName {
			continue
		}

		desired := c.VolumeMounts
		for _, m := range mounts {
			desired = withVolumeMount(desired, m)
		}

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

	var env []corev1.EnvVar
	if watch {
		args = append(args, "--watch", "--interval", sidecarWatchInterval)
		// Only the sidecar needs to self-identify: drift detection uses
		// these to attribute a best-effort Event to this Pod. The
		// one-shot init container never runs the drift-detection path.
		env = []corev1.EnvVar{
			{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
			{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
		}
	}

	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	runAsNonRoot := true

	return corev1.Container{
		Name: name,
		// Explicit: a ":latest" tag defaults to PullAlways, which fails
		// against a locally-loaded, unpushed image.
		Image:           cfg.InjectImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            args,
		Env:             env,
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: mountPath},
		},
		// A namespace with a ResourceQuota on cpu or memory rejects any Pod
		// whose containers omit requests and limits, which would make merely
		// annotating a Pod there un-creatable.
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("16Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		// Set explicitly rather than inherited: a consumer Pod under the
		// restricted Pod Security standard rejects containers that leave these
		// unset, and these containers are added without the consumer's
		// knowledge.
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
			RunAsNonRoot:             &runAsNonRoot,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
}
