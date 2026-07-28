package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TenantConfigSpec defines the desired state of TenantConfig.
type TenantConfigSpec struct {
	// RuntimeConfig holds arbitrary key-value pairs made available to
	// services at runtime via Muninn Query API (TENANT.runtime key),
	// Values are strings; consumers interpret types themselves.
	// +optional
	RuntimeConfig map[string]string `json:"runtimeconfig,omitempty"`
}

// TenantConfigStatus defines the observed state of TenantConfig.
type TenantConfigStatus struct {
	// Conditions represent the latest observations of the resource state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=tc
// +kubebuilder:printcoumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// TenantConfig holds runtime configuration for a single tenant.
// It is namespace-scoped and lives in the tenantś dedicted namespace.
type TenantConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantConfigSpec   `json:"spec,omitempty"`
	Status TenantConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantConfigList contains a lit of TenantConfig.
type TenantConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TenantConfig{}, &TenantConfigList{})
}
