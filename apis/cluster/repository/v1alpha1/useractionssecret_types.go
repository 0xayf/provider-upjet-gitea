// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// UserActionsSecretObservation reflects observed state from Gitea.
type UserActionsSecretObservation struct {
	// CreatedAt is the time the secret was created in Gitea.
	CreatedAt *string `json:"createdAt,omitempty"`

	// Existed is set to true once the controller has successfully PUT the
	// secret to Gitea. Gitea's /user/actions/secrets endpoint does not
	// expose a list or get operation, so the controller cannot otherwise
	// detect existence of a previously-created user-scoped secret.
	Existed *bool `json:"existed,omitempty"`

	// ValueHash is a SHA-256 of the secret value last pushed to Gitea. Used
	// to detect value rotation so the controller can re-push the new
	// value to Gitea via Update.
	ValueHash *string `json:"valueHash,omitempty"`
}

// UserActionsSecretParameters define the desired state of a user-scoped Gitea
// Actions secret. The user is implied by the credentials of the referenced
// ProviderConfig - Gitea's /user/actions/secrets endpoint operates on the
// authenticated user, so each UserActionsSecret must reference a
// ProviderConfig that authenticates as the target user.
type UserActionsSecretParameters struct {
	// SecretName is the name of the Actions secret as it appears to workflows
	// run in repositories owned by the authenticated user.
	// +kubebuilder:validation:Optional
	SecretName *string `json:"secretName,omitempty"`

	// SecretValueSecretRef references a Kubernetes secret containing the
	// value of the Actions secret.
	SecretValueSecretRef v1.SecretKeySelector `json:"secretValueSecretRef"`
}

// UserActionsSecretSpec defines the desired state of UserActionsSecret.
type UserActionsSecretSpec struct {
	v1.ResourceSpec `json:",inline"`
	ForProvider     UserActionsSecretParameters `json:"forProvider"`
}

// UserActionsSecretStatus defines the observed state of UserActionsSecret.
type UserActionsSecretStatus struct {
	v1.ResourceStatus `json:",inline"`
	AtProvider        UserActionsSecretObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// UserActionsSecret manages a Gitea Actions secret on the authenticated
// user's account.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,gitea}
type UserActionsSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.secretName)",message="spec.forProvider.secretName is a required parameter"
	Spec   UserActionsSecretSpec   `json:"spec"`
	Status UserActionsSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserActionsSecretList contains a list of UserActionsSecrets.
type UserActionsSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UserActionsSecret `json:"items"`
}

// UserActionsSecret type metadata.
var (
	UserActionsSecret_Kind             = "UserActionsSecret"
	UserActionsSecret_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: UserActionsSecret_Kind}.String()
	UserActionsSecret_KindAPIVersion   = UserActionsSecret_Kind + "." + CRDGroupVersion.String()
	UserActionsSecret_GroupVersionKind = CRDGroupVersion.WithKind(UserActionsSecret_Kind)
)

func init() {
	SchemeBuilder.Register(&UserActionsSecret{}, &UserActionsSecretList{})
}
