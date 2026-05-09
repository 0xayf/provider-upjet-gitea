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

// TeamObservation reflects observed Gitea state.
type TeamObservation struct {
	// ID is Gitea's numeric team identifier (the canonical primary key).
	ID *int64 `json:"id,omitempty"`

	// Permission is Gitea's legacy single-permission view, derived from
	// units_map. We surface it as observation only — set permissions via
	// spec.forProvider.unitsMap.
	Permission *string `json:"permission,omitempty"`

	// Units is the list of unit names the team has any access to.
	Units []string `json:"units,omitempty"`

	// UnitsMap is the per-unit permission level Gitea reports.
	UnitsMap map[string]string `json:"unitsMap,omitempty"`
}

// TeamParameters is the desired state of a Gitea team. unitsMap is required —
// the legacy `permission` + `units` shape is intentionally not exposed here.
type TeamParameters struct {
	// Name is the team's name within the organisation. Immutable on the
	// Gitea side after creation; renaming requires delete + recreate.
	// +kubebuilder:validation:Optional
	Name *string `json:"name,omitempty"`

	// Organisation is the org that owns the team.
	// +kubebuilder:validation:Optional
	Organisation *string `json:"organisation,omitempty"`

	// Description is a free-form team description shown in the Gitea UI.
	// +kubebuilder:validation:Optional
	Description *string `json:"description,omitempty"`

	// IncludeAllRepositories grants the team access to every current and
	// future repository in the organisation. Mutually exclusive with
	// Repositories — when true, Repositories is ignored.
	// +kubebuilder:validation:Optional
	IncludeAllRepositories *bool `json:"includeAllRepositories,omitempty"`

	// Repositories is the explicit list of repos the team's permissions
	// apply to. Empty when IncludeAllRepositories is true.
	// +kubebuilder:validation:Optional
	Repositories []string `json:"repositories,omitempty"`

	// CanCreateOrgRepo lets team members create new repos under the org.
	// +kubebuilder:validation:Optional
	CanCreateOrgRepo *bool `json:"canCreateOrgRepo,omitempty"`

	// UnitsMap is the per-unit permission map. Keys must be valid Gitea
	// team unit names; values must be one of `none`, `read`, `write`,
	// `admin`. A unit absent from the map is `none` for that team.
	//
	// The full set of recognised keys is:
	//   repo.code, repo.issues, repo.ext_issues,
	//   repo.wiki, repo.ext_wiki,
	//   repo.pulls, repo.releases, repo.projects,
	//   repo.packages, repo.actions
	UnitsMap map[string]string `json:"unitsMap"`
}

// TeamSpec defines the desired state of Team.
type TeamSpec struct {
	v2.ManagedResourceSpec `json:",inline"`
	ForProvider            TeamParameters `json:"forProvider"`
}

// TeamStatus defines the observed state of Team.
type TeamStatus struct {
	v1.ResourceStatus `json:",inline"`
	AtProvider        TeamObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// Team manages a Gitea organisation team with explicit per-unit permissions
// (units_map). Replaces the upjet-generated Team resource so the controller
// can drive Gitea's per-unit permission model directly, including
// repo.packages / repo.actions which the upstream Terraform provider's
// `units` enum does not honour.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,gitea}
type Team struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.name)",message="spec.forProvider.name is a required parameter"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || has(self.forProvider.organisation)",message="spec.forProvider.organisation is a required parameter"
	// +kubebuilder:validation:XValidation:rule="!('*' in self.managementPolicies || 'Create' in self.managementPolicies || 'Update' in self.managementPolicies) || (has(self.forProvider.unitsMap) && size(self.forProvider.unitsMap) > 0)",message="spec.forProvider.unitsMap must have at least one entry"
	Spec   TeamSpec   `json:"spec"`
	Status TeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Teams.
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Team `json:"items"`
}

// Team type metadata.
var (
	Team_Kind             = "Team"
	Team_GroupKind        = schema.GroupKind{Group: CRDGroup, Kind: Team_Kind}.String()
	Team_KindAPIVersion   = Team_Kind + "." + CRDGroupVersion.String()
	Team_GroupVersionKind = CRDGroupVersion.WithKind(Team_Kind)
)

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}
