// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/cluster/gitea/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

const (
	errNotTeam       = "managed resource is not a Team"
	errResolveCreds  = "cannot resolve gitea credentials"
	errMissingName   = "spec.forProvider.name is required"
	errMissingOrg    = "spec.forProvider.organisation is required"
	errMissingUnits  = "spec.forProvider.unitsMap must have at least one entry"
	errInvalidUnits  = "spec.forProvider.unitsMap contains invalid keys or levels"
	errGet           = "cannot get team from gitea"
	errCreate        = "cannot create team in gitea"
	errUpdate        = "cannot update team in gitea"
	errDelete        = "cannot delete team in gitea"
	errReposAttach   = "cannot attach repositories to team"
	errReposDetach   = "cannot detach repositories from team"
)

// Setup wires the controller for cluster Team resources.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.Team_GroupVersionKind.String())
	r := managed.NewReconciler(mgr,
		xpresource.ManagedKind(v1alpha1.Team_GroupVersionKind),
		managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
	)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.Team{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated registers Setup behind the controller-engine gate.
func SetupGated(mgr ctrl.Manager, o tjcontroller.Options) error {
	o.Options.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "unable to setup reconciler", "gvk", v1alpha1.Team_GroupVersionKind.String())
		}
	}, v1alpha1.Team_GroupVersionKind)
	return nil
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return nil, errors.New(errNotTeam)
	}
	creds, err := clients.ResolveGiteaCreds(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errResolveCreds)
	}
	return &external{kube: c.kube, api: clients.NewGiteaTeamClient(creds)}, nil
}

type external struct {
	kube client.Client
	api  clients.TeamAPI
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotTeam)
	}
	if err := validateSpec(&cr.Spec.ForProvider); err != nil {
		return managed.ExternalObservation{}, err
	}

	got, err := e.api.Get(ctx, *cr.Spec.ForProvider.Organisation, *cr.Spec.ForProvider.Name)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGet)
	}
	if got == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	// Mirror canonical state into atProvider so consumers can read teamId
	// (used by older membership claims) and the resolved unitsMap. ID is
	// surfaced as a decimal string for cross-version compatibility with the
	// upjet-shape data already present in clusters.
	idStr := strconv.FormatInt(got.ID, 10)
	cr.Status.AtProvider.ID = &idStr
	if got.UnitsMap != nil {
		um := make(map[string]string, len(got.UnitsMap))
		for k, v := range got.UnitsMap {
			um[k] = string(v)
		}
		cr.Status.AtProvider.UnitsMap = um
	}
	meta.SetExternalName(cr, strconv.FormatInt(got.ID, 10))

	upToDate := isTeamUpToDate(&cr.Spec.ForProvider, got)
	if upToDate {
		// Repositories attachment may still drift even when the team itself
		// matches; check separately.
		reposUpToDate, err := e.reposUpToDate(ctx, got, &cr.Spec.ForProvider)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		upToDate = reposUpToDate
	}

	cr.SetConditions(xpv1.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotTeam)
	}
	if err := validateSpec(&cr.Spec.ForProvider); err != nil {
		return managed.ExternalCreation{}, err
	}

	params := paramsFromSpec(&cr.Spec.ForProvider)
	created, err := e.api.Create(ctx, *cr.Spec.ForProvider.Organisation, params)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}
	idStr := strconv.FormatInt(created.ID, 10)
	cr.Status.AtProvider.ID = &idStr
	meta.SetExternalName(cr, idStr)

	// Attach repositories if not includes_all_repositories.
	if !boolValue(cr.Spec.ForProvider.IncludeAllRepositories) {
		for _, r := range cr.Spec.ForProvider.Repositories {
			if err := e.api.AddRepository(ctx, created.ID, r); err != nil {
				return managed.ExternalCreation{}, errors.Wrap(err, errReposAttach)
			}
		}
	}
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotTeam)
	}
	if err := validateSpec(&cr.Spec.ForProvider); err != nil {
		return managed.ExternalUpdate{}, err
	}
	if cr.Status.AtProvider.ID == nil {
		// Re-fetch by name to recover the ID if status was wiped.
		got, err := e.api.Get(ctx, *cr.Spec.ForProvider.Organisation, *cr.Spec.ForProvider.Name)
		if err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errGet)
		}
		if got == nil {
			return managed.ExternalUpdate{}, errors.New("team not found during update")
		}
		idStr := strconv.FormatInt(got.ID, 10)
		cr.Status.AtProvider.ID = &idStr
	}
	id, err := strconv.ParseInt(*cr.Status.AtProvider.ID, 10, 64)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "status.atProvider.id is not a valid integer")
	}

	params := paramsFromSpec(&cr.Spec.ForProvider)
	updated, err := e.api.Update(ctx, id, params)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}
	if updated.UnitsMap != nil {
		um := make(map[string]string, len(updated.UnitsMap))
		for k, v := range updated.UnitsMap {
			um[k] = string(v)
		}
		cr.Status.AtProvider.UnitsMap = um
	}

	// Reconcile repositories list: add missing, remove extra. Skipped when
	// includeAllRepositories is true since Gitea owns the list in that case.
	if !boolValue(cr.Spec.ForProvider.IncludeAllRepositories) {
		current, err := e.api.ListRepositories(ctx, id)
		if err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errGet)
		}
		desired := append([]string(nil), cr.Spec.ForProvider.Repositories...)
		sort.Strings(desired)
		toAdd, toRemove := diffStringSets(desired, current)
		for _, r := range toAdd {
			if err := e.api.AddRepository(ctx, id, r); err != nil {
				return managed.ExternalUpdate{}, errors.Wrap(err, errReposAttach)
			}
		}
		for _, r := range toRemove {
			if err := e.api.RemoveRepository(ctx, id, r); err != nil {
				return managed.ExternalUpdate{}, errors.Wrap(err, errReposDetach)
			}
		}
	}
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Team)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotTeam)
	}
	if cr.Status.AtProvider.ID == nil {
		// Best-effort lookup so a team created out-of-band can still be
		// deleted by name.
		if cr.Spec.ForProvider.Organisation == nil || cr.Spec.ForProvider.Name == nil {
			return managed.ExternalDelete{}, nil
		}
		got, err := e.api.Get(ctx, *cr.Spec.ForProvider.Organisation, *cr.Spec.ForProvider.Name)
		if err != nil {
			return managed.ExternalDelete{}, errors.Wrap(err, errGet)
		}
		if got == nil {
			return managed.ExternalDelete{}, nil
		}
		idStr := strconv.FormatInt(got.ID, 10)
		cr.Status.AtProvider.ID = &idStr
	}
	id, err := strconv.ParseInt(*cr.Status.AtProvider.ID, 10, 64)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, "status.atProvider.id is not a valid integer")
	}
	if err := e.api.Delete(ctx, id); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

// reposUpToDate compares the current repo attachments to what's desired.
// Returns true when in sync, false when there's drift to reconcile.
func (e *external) reposUpToDate(ctx context.Context, got *clients.TeamResource, p *v1alpha1.TeamParameters) (bool, error) {
	if boolValue(p.IncludeAllRepositories) {
		// Gitea manages the repo list; we don't compare.
		return got.IncludesAllRepositories, nil
	}
	if got.IncludesAllRepositories {
		return false, nil
	}
	current, err := e.api.ListRepositories(ctx, got.ID)
	if err != nil {
		return false, errors.Wrap(err, errGet)
	}
	desired := append([]string(nil), p.Repositories...)
	sort.Strings(desired)
	return reflect.DeepEqual(desired, current), nil
}

// validateSpec runs the spec-side checks before any API call. Returning an
// early error here keeps Gitea's responses out of the failure mode for
// missing or malformed inputs.
func validateSpec(p *v1alpha1.TeamParameters) error {
	if p.Name == nil || *p.Name == "" {
		return errors.New(errMissingName)
	}
	if p.Organisation == nil || *p.Organisation == "" {
		return errors.New(errMissingOrg)
	}
	if len(p.UnitsMap) == 0 {
		return errors.New(errMissingUnits)
	}
	for unit, level := range p.UnitsMap {
		if _, ok := clients.ValidTeamUnits[unit]; !ok {
			return fmt.Errorf("%s: %q", errInvalidUnits, unit)
		}
		if !clients.TeamPermissionLevel(level).IsValid() {
			return fmt.Errorf("%s: %s=%q", errInvalidUnits, unit, level)
		}
	}
	return nil
}

// paramsFromSpec translates the XR spec into the Gitea API request shape.
// We expand UnitsMap to cover every recognised unit with an explicit level
// (defaulting absent units to "none") so PATCH calls fully replace the
// team's permission state — Gitea's PATCH only touches the keys we send.
func paramsFromSpec(p *v1alpha1.TeamParameters) clients.TeamParams {
	full := make(map[string]clients.TeamPermissionLevel, len(clients.ValidTeamUnits))
	for unit := range clients.ValidTeamUnits {
		full[unit] = clients.TeamPermNone
	}
	for unit, level := range p.UnitsMap {
		full[unit] = clients.TeamPermissionLevel(level)
	}
	return clients.TeamParams{
		Name:                    deref(p.Name),
		Description:             deref(p.Description),
		IncludesAllRepositories: boolValue(p.IncludeAllRepositories),
		CanCreateOrgRepo:        boolValue(p.CanCreateOrgRepo),
		UnitsMap:                full,
	}
}

// isTeamUpToDate reports whether the spec's team-level fields match Gitea.
// Repository membership is checked separately.
func isTeamUpToDate(p *v1alpha1.TeamParameters, got *clients.TeamResource) bool {
	if deref(p.Description) != got.Description {
		return false
	}
	if boolValue(p.IncludeAllRepositories) != got.IncludesAllRepositories {
		return false
	}
	if boolValue(p.CanCreateOrgRepo) != got.CanCreateOrgRepo {
		return false
	}
	desired := paramsFromSpec(p).UnitsMap
	observed := got.UnitsMap
	if len(desired) != len(observed) {
		return false
	}
	for k, v := range desired {
		if observed[k] != v {
			return false
		}
	}
	return true
}

// diffStringSets returns elements in desired-not-in-current (toAdd) and
// elements in current-not-in-desired (toRemove). Both lists must be sorted.
func diffStringSets(desired, current []string) (toAdd, toRemove []string) {
	d := make(map[string]struct{}, len(desired))
	for _, x := range desired {
		d[x] = struct{}{}
	}
	c := make(map[string]struct{}, len(current))
	for _, x := range current {
		c[x] = struct{}{}
	}
	for _, x := range desired {
		if _, ok := c[x]; !ok {
			toAdd = append(toAdd, x)
		}
	}
	for _, x := range current {
		if _, ok := d[x]; !ok {
			toRemove = append(toRemove, x)
		}
	}
	return toAdd, toRemove
}

func boolValue(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
