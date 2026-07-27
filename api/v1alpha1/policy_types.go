package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// JWTConfig holds JWT validation settings for a tenant.
type JWTConfig struct {
	// IssuerAllowList is an option list of accepted JWT issuers.
	// When empty, issuer validation is skipped.
	// +optional
	IssuerAllowList []string `json:"issuerAllowList,omitempty"`

	// SubjectClaim is the JWT claim used as the subject identifier.
	// Defaults to "sub" when empty.
	// +optional
	SubjectClaim string `json:"subjectClaim,omitempty"`

	// ScopesClaim is the JWT claim used to extract scopes.
	// Defaults to "scp" when empty.
	// +optional
	ScopesClaim string `json:"scopesClaim,omitempty"`
}

// Binding maps a subject to a set of permissions.
type Binding struct {
	// Subject is the identity being granted permissions (e.g. a user or service account).
	Subject string `json:"subject"`

	// Permissions is the list of permission granted to the subject.
	// +optional
	Permissions []string `json:"permissions,omitempty"`
}

// RoleBinding maps a role name to a set of permissions.
type RoleBinding struct {
	// Role is the name of the role.
	Role string `json:"role"`

	// Permissions is the list of permissions granted to members of this role.
	// +optional
	Permissions []string `json:"permissions,omitempty"`
}

// PolicySpec defines the desired state of Policy.
type PolicySpec struct {
	// JWT holds JWT validation configuration for this tenant.
	// +optional
	JWT JWTConfig `json:"jwt,omitempty"`

	// Bindings grants permissions directly to subjects.
	// +optional
	Bindings []Binding `json:"bindings,omitempty"`

	// RoleBindings grants permissions to named roles.
	// +optional
	RoleBindings []RoleBinding `json:"roleBindings,omitempty"`
}

// PolicyStatus defines the observed state of Policy.
type PolicyStatus struct {
	// Conditions represent the latest observations of the resource state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=pol
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Policy holds authorization configuration for a single tenant.
// It is namespace-scoped and lives in the tenant's dedicated namespace.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicySpec   `json:"spec,omitempty"`
	Status PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyList contains a list of Policy.
type PolicyList struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Items             []Policy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
}
