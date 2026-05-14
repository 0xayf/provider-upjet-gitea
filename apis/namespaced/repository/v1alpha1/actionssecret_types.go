// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// ActionsSecretObservation reflects observed state from Gitea.
type ActionsSecretObservation struct {
	// CreatedAt is the time the secret was created in Gitea.
	CreatedAt *string `json:"createdAt,omitempty"`

	// ValueHash is a SHA-256 of the secret value last pushed to Gitea. The
	// controller compares this against the current source secret's hash to
	// detect when a value rotation should re-push to Gitea (Gitea's API
	// does not expose the secret value for a direct comparison).
	ValueHash *string `json:"valueHash,omitempty"`
}

// ActionsSecretParameters define the desired state of a repository-scoped
// Gitea Actions secret.
type ActionsSecretParameters struct {
	// RepositoryOwner is the user or organisation that owns the repo.
	// +kubebuilder:validation:Optional
	RepositoryOwner *string `json:"repositoryOwner,omitempty"`

	// Repository is the repo name within RepositoryOwner.
	// +kubebuilder:validation:Optional
	Repository *string `json:"repository,omitempty"`

	// SecretName is the name of the Actions secret as it appears to workflows.
	// +kubebuilder:validation:Optional
	SecretName *string `json:"secretName,omitempty"`

	// SecretValueSecretRef references an in-namespace Kubernetes secret
	// containing the value of the Actions secret.
	SecretValueSecretRef v1.LocalSecretKeySelector `json:"secretValueSecretRef"`
}

// ActionsSecretSpec defines the desired state of ActionsSecret.
type ActionsSecretSpec struct {
	v2.ManagedResourceSpec `json:",inline"`
	ForProvider            ActionsSecretParameters `json:"forProvider"`
}

// ActionsSecretStatus defines the observed state of ActionsSecret.
type ActionsSecretStatus struct {
	v1.ResourceStatus `json:",inline"`
	AtProvider        ActionsSecretObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// ActionsSecret manages a repository-scoped Gitea Actions secret. The
// controller detects value rotation by hashing the source secret and
// comparing against status.atProvider.valueHash on each Observe, since
// Gitea's API never exposes secret values for direct comparison.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gitea}
type ActionsSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.repositoryOwner)",message="spec.forProvider.repositoryOwner is a required parameter"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.repository)",message="spec.forProvider.repository is a required parameter"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.secretName)",message="spec.forProvider.secretName is a required parameter"
	Spec   ActionsSecretSpec   `json:"spec"`
	Status ActionsSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ActionsSecretList contains a list of ActionsSecrets.
type ActionsSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ActionsSecret `json:"items"`
}

// ActionsSecret type metadata.
var (
	ActionsSecret_Kind             = "ActionsSecret"
	ActionsSecret_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: ActionsSecret_Kind}.String()
	ActionsSecret_KindAPIVersion   = ActionsSecret_Kind + "." + CRDGroupVersion.String()
	ActionsSecret_GroupVersionKind = CRDGroupVersion.WithKind(ActionsSecret_Kind)
)

func init() {
	SchemeBuilder.Register(&ActionsSecret{}, &ActionsSecretList{})
}
