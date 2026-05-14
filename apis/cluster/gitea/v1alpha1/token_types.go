// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// TokenObservation reflects observed state from Gitea.
type TokenObservation struct {
	// ID is Gitea's numeric token identifier. Stored as a string here for
	// consistency with the upjet-shape MRs already on cluster.
	ID *string `json:"id,omitempty"`

	// Scopes is the set Gitea reports for this token. The controller
	// compares this against spec.forProvider.scopes to detect scope drift.
	Scopes []string `json:"scopes,omitempty"`

	// LastEight is the last eight characters of the token value as
	// returned at creation time. Useful for identifying which token a
	// claim corresponds to in the Gitea UI.
	LastEight *string `json:"lastEight,omitempty"`

	// RotatedAt is the timestamp at which the controller last performed
	// a delete-and-recreate to reconcile a scope-set drift. Tokens
	// cannot be patched in place — Gitea has no PATCH endpoint — so any
	// scope change requires re-minting the token value.
	RotatedAt *metav1.Time `json:"rotatedAt,omitempty"`
}

// TokenParameters defines the desired state of a Gitea access token. The
// token is minted under the user identified by the credentials in the
// referenced ProviderConfig (Gitea's /users/{username}/tokens endpoint
// accepts only HTTP Basic auth as that user).
type TokenParameters struct {
	// Name is the token's display name. Immutable on the Gitea side after
	// creation; changing it requires destroy + recreate, which the
	// controller handles transparently.
	// +kubebuilder:validation:Optional
	Name *string `json:"name,omitempty"`

	// Scopes is the set of OAuth scopes the token authorises. Valid
	// scopes are documented in Gitea's API reference. Changing this set
	// triggers a delete-and-recreate (Gitea has no PATCH for tokens);
	// the controller updates the connection secret with the new value,
	// so downstream consumers see the rotation automatically.
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}

// TokenSpec defines the desired state of Token.
type TokenSpec struct {
	v1.ResourceSpec `json:",inline"`
	ForProvider     TokenParameters `json:"forProvider"`
}

// TokenStatus defines the observed state of Token.
type TokenStatus struct {
	v1.ResourceStatus `json:",inline"`
	AtProvider        TokenObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// Token manages a Gitea access token. The hand-written controller talks
// to /users/{username}/tokens directly via HTTP Basic auth (the only
// auth mode Gitea accepts on this endpoint family), captures the token
// value at creation time, and re-mints on scope drift so PAT scope
// rotation becomes a plain spec edit.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,gitea}
type Token struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.name)",message="spec.forProvider.name is a required parameter"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || (has(self.forProvider.scopes) && size(self.forProvider.scopes) > 0)",message="spec.forProvider.scopes must contain at least one scope"
	Spec   TokenSpec   `json:"spec"`
	Status TokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TokenList contains a list of Tokens.
type TokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Token `json:"items"`
}

// Token type metadata.
var (
	Token_Kind             = "Token"
	Token_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: Token_Kind}.String()
	Token_KindAPIVersion   = Token_Kind + "." + CRDGroupVersion.String()
	Token_GroupVersionKind = CRDGroupVersion.WithKind(Token_Kind)
)

func init() {
	SchemeBuilder.Register(&Token{}, &TokenList{})
}
