package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/api/meta/v1"

// TenantPhase represent the lifecycle phase of a Tenant resource.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Suspended;Deleting
type TenantPhase string

// TenantSpec defines the desired state of Tenant.
type TenantSpec struct {
	// TenantID is the stable, globally unique identifier for the tenant.
	// Immutable once set.
	// +optional
	TenantID string `json:"tenantID,omitempty"`

	// DisplayName is a human-friendly name for the tenant.
	// +optional
	DisplayName string `json:"displayName,omitempty"`
}

// CloudResources tracks external cloud resources provisioned for this tenant.
type CloudResources struct {
	// IdentityPoolID is the ID of the identity provider pool for this tenant.
	// +optional
	IdentityPoolID string `json:"identityPoolID,omitempty"`

	// IdentityPoolARN is the ARN or full resource identifier of the identity pool.
	// +optional
	IdentityPoolARN string `json:"identityPoolARN,omitempty"`

	// IdentityClientID is the client application ID registered with the identity provider.
	// +optional
	IdentityClientID string `json:"identityClientID,omitempty"`

	// IdentityDomain is the tenant-scoped domain for the identity provider.
	// +optional
	IdentityDomain string `json:"identityDomain,omitempty"`

	// StorageBucketName is the name of the storage bucket provisioned for this tenant.
	// +optional
	StorageBucketName string `json:"storageBucketName,omitempty"`

	// StorageBucketARN is the ARN or full resource identifier of the storage bucket.
	// +optional
	StorageBucketARN string `json:"storageBucketARN,omitempty"`
}

// TenantStatus defines the observed state of Tenant.
type TenantStatus struct {
	// Phase indicates the current lifecycle phase of the tenant.
	// +optional
	Phase TenantPhase `json:"phase,omitempty"`

	// Namespace is the dedicted kubernetes namespace for this tenant.
	// Format: tenant-<tenantID>
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// CloudResources tracks external resources provisioned for this tenant.
	// +optional
	CloudResources CloudResources `json:"cloudResouces,omitempty"`

	// Conditions represent the latest overvations of the resource state.
	// +optional
	Coditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortname=tn
// +kubebuilder:printcolumn:name="Tenant ID",type=string,JSONPath=`.spec.tenantID`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tenant is the schema for the tenants API.
// It represents a single tenant in the platform.
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantList contains a list of Tenant.
type TenantList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Items             []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
