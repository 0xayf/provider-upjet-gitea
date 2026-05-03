// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// OrgActionsSecretObservation reflects observed state from Gitea.
type OrgActionsSecretObservation struct {
	// CreatedAt is the time the secret was created in Gitea.
	CreatedAt *string `json:"createdAt,omitempty"`

	// ValueHash is a SHA-256 of the secret value last pushed to Gitea. The
	// controller compares this against the current source secret's hash to
	// detect when a value rotation should re-push to Gitea (Gitea's API
	// does not expose the secret value for a direct comparison).
	ValueHash *string `json:"valueHash,omitempty"`
}

// OrgActionsSecretParameters define the desired state of an organisation-scoped
// Gitea Actions secret.
type OrgActionsSecretParameters struct {
	// Org is the name of the Gitea organisation.
	// +kubebuilder:validation:Optional
	Org *string `json:"org,omitempty"`

	// SecretName is the name of the Actions secret as it appears to workflows.
	// +kubebuilder:validation:Optional
	SecretName *string `json:"secretName,omitempty"`

	// SecretValueSecretRef references a Kubernetes secret containing the
	// value of the Actions secret.
	SecretValueSecretRef v1.SecretKeySelector `json:"secretValueSecretRef"`
}

// OrgActionsSecretSpec defines the desired state of OrgActionsSecret.
type OrgActionsSecretSpec struct {
	v1.ResourceSpec `json:",inline"`
	ForProvider     OrgActionsSecretParameters `json:"forProvider"`
}

// OrgActionsSecretStatus defines the observed state of OrgActionsSecret.
type OrgActionsSecretStatus struct {
	v1.ResourceStatus `json:",inline"`
	AtProvider        OrgActionsSecretObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// OrgActionsSecret manages an organisation-scoped Gitea Actions secret.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,gitea}
type OrgActionsSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.org)",message="spec.forProvider.org is a required parameter"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.secretName)",message="spec.forProvider.secretName is a required parameter"
	Spec   OrgActionsSecretSpec   `json:"spec"`
	Status OrgActionsSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OrgActionsSecretList contains a list of OrgActionsSecrets.
type OrgActionsSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OrgActionsSecret `json:"items"`
}

// OrgActionsSecret type metadata.
var (
	OrgActionsSecret_Kind             = "OrgActionsSecret"
	OrgActionsSecret_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: OrgActionsSecret_Kind}.String()
	OrgActionsSecret_KindAPIVersion   = OrgActionsSecret_Kind + "." + CRDGroupVersion.String()
	OrgActionsSecret_GroupVersionKind = CRDGroupVersion.WithKind(OrgActionsSecret_Kind)
)

func init() {
	SchemeBuilder.Register(&OrgActionsSecret{}, &OrgActionsSecretList{})
}
