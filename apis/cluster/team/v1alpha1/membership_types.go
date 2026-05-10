// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
)

// TeamRef identifies a Gitea team by its organisation and name. The
// controller resolves this to the team's numeric ID at reconcile time.
type TeamRef struct {
	// Org is the Gitea organisation that owns the team.
	Org string `json:"org"`

	// Name is the team's name within the organisation.
	Name string `json:"name"`
}

// MembershipObservation reflects observed Gitea state.
type MembershipObservation struct {
	// TeamID is the numeric team ID resolved from spec.forProvider.team
	// at the last successful reconcile. Surfaced to make it easy to
	// cross-reference Gitea's UI.
	TeamID *int64 `json:"teamId,omitempty"`
}

// MembershipParameters define the desired membership of a Gitea team.
//
// Two ways to point at the team: the new `team` ref by org and name, or the
// legacy `teamId` numeric ID carried over from the upjet shape for
// back-compat. When both are set, `team` wins. Existing MRs created by
// upstream provider v0.2.x continue to work as-is via `teamId`.
type MembershipParameters struct {
	// Team identifies the Gitea team by org + name. The controller
	// resolves this to the numeric ID at reconcile time, so callers
	// don't need to know Gitea's auto-assigned IDs.
	// +kubebuilder:validation:Optional
	Team *TeamRef `json:"team,omitempty"`

	// TeamID is the legacy numeric team ID. Set this *or* `team`, not
	// both. When both are set, `team` wins. Stored as float64 to match
	// the shape on-cluster MRs already hold (the upstream Terraform
	// provider treats numeric IDs as floats).
	// +kubebuilder:validation:Optional
	TeamID *float64 `json:"teamId,omitempty"`

	// Username is the Gitea user to add to the team.
	// +kubebuilder:validation:Optional
	Username *string `json:"username,omitempty"`
}

// MembershipSpec defines the desired state of Membership.
type MembershipSpec struct {
	v1.ResourceSpec `json:",inline"`
	ForProvider     MembershipParameters `json:"forProvider"`
}

// MembershipStatus defines the observed state of Membership.
type MembershipStatus struct {
	v1.ResourceStatus `json:",inline"`
	AtProvider        MembershipObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// Membership manages a Gitea user's membership in a team. The team is
// referenced by org + name; the controller resolves to Gitea's numeric ID
// at reconcile time, so claims don't need to depend on Crossplane sibling
// XRs to find the ID.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,gitea}
type Membership struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.team) || has(self.forProvider.teamId)",message="one of spec.forProvider.team or spec.forProvider.teamId must be set"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.username)",message="spec.forProvider.username is a required parameter"
	Spec   MembershipSpec   `json:"spec"`
	Status MembershipStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MembershipList contains a list of Memberships.
type MembershipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Membership `json:"items"`
}

// Membership type metadata.
var (
	Membership_Kind             = "Membership"
	Membership_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: Membership_Kind}.String()
	Membership_KindAPIVersion   = Membership_Kind + "." + CRDGroupVersion.String()
	Membership_GroupVersionKind = CRDGroupVersion.WithKind(Membership_Kind)
)

func init() {
	SchemeBuilder.Register(&Membership{}, &MembershipList{})
}
