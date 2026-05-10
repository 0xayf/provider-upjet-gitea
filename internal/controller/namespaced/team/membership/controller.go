// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package membership

import (
	"context"
	"fmt"

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

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/namespaced/team/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

const (
	errNotMembership   = "managed resource is not a Membership"
	errResolveCreds    = "cannot resolve gitea credentials"
	errMissingTeam     = "spec.forProvider.team (org and name) is required"
	errMissingUsername = "spec.forProvider.username is required"
	errResolveTeam     = "cannot resolve team by org and name"
	errCheckMember     = "cannot check team membership in gitea"
	errAddMember       = "cannot add team member in gitea"
	errRemoveMember    = "cannot remove team member in gitea"
)

// ErrTeamNotFound is returned by resolveTeamID when the named team does not
// exist on Gitea. Callers can treat this as "nothing to clean up" during
// Delete to keep the operation idempotent.
var ErrTeamNotFound = errors.New("team not found")

// Setup wires the controller for cluster Membership resources.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.Membership_GroupVersionKind.String())
	r := managed.NewReconciler(mgr,
		xpresource.ManagedKind(v1alpha1.Membership_GroupVersionKind),
		managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
	)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.Membership{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated registers Setup behind the controller-engine gate.
func SetupGated(mgr ctrl.Manager, o tjcontroller.Options) error {
	o.Options.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "unable to setup reconciler", "gvk", v1alpha1.Membership_GroupVersionKind.String())
		}
	}, v1alpha1.Membership_GroupVersionKind)
	return nil
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Membership)
	if !ok {
		return nil, errors.New(errNotMembership)
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

// resolveTeamID returns the numeric Gitea team ID for this Membership,
// preferring the modern spec.team {org, name} ref but falling back to the
// legacy spec.teamId for back-compat with v0.2.x MRs. Cached on
// status.atProvider.teamId after the first successful resolve.
func (e *external) resolveTeamID(ctx context.Context, cr *v1alpha1.Membership) (int64, error) {
	if cr.Status.AtProvider.TeamID != nil && *cr.Status.AtProvider.TeamID > 0 {
		return *cr.Status.AtProvider.TeamID, nil
	}
	// Modern path: org + name -> numeric ID via Gitea API.
	if cr.Spec.ForProvider.Team != nil && cr.Spec.ForProvider.Team.Org != "" && cr.Spec.ForProvider.Team.Name != "" {
		got, err := e.api.Get(ctx, cr.Spec.ForProvider.Team.Org, cr.Spec.ForProvider.Team.Name)
		if err != nil {
			return 0, errors.Wrap(err, errResolveTeam)
		}
		if got == nil {
			return 0, fmt.Errorf("%w: %s/%s", ErrTeamNotFound, cr.Spec.ForProvider.Team.Org, cr.Spec.ForProvider.Team.Name)
		}
		id := got.ID
		cr.Status.AtProvider.TeamID = &id
		return id, nil
	}
	// Legacy path: numeric teamId from upjet shape. No name to look up;
	// the ID is the source of truth.
	if cr.Spec.ForProvider.TeamID != nil && *cr.Spec.ForProvider.TeamID > 0 {
		id := int64(*cr.Spec.ForProvider.TeamID)
		cr.Status.AtProvider.TeamID = &id
		return id, nil
	}
	return 0, errors.New(errMissingTeam)
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Membership)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotMembership)
	}
	if cr.Spec.ForProvider.Username == nil || *cr.Spec.ForProvider.Username == "" {
		return managed.ExternalObservation{}, errors.New(errMissingUsername)
	}

	id, err := e.resolveTeamID(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	isMember, err := e.api.IsMember(ctx, id, *cr.Spec.ForProvider.Username)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errCheckMember)
	}
	if !isMember {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	meta.SetExternalName(cr, fmt.Sprintf("%d:%s", id, *cr.Spec.ForProvider.Username))
	cr.SetConditions(xpv1.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Membership)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotMembership)
	}
	if cr.Spec.ForProvider.Username == nil || *cr.Spec.ForProvider.Username == "" {
		return managed.ExternalCreation{}, errors.New(errMissingUsername)
	}
	id, err := e.resolveTeamID(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if err := e.api.AddMember(ctx, id, *cr.Spec.ForProvider.Username); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errAddMember)
	}
	meta.SetExternalName(cr, fmt.Sprintf("%d:%s", id, *cr.Spec.ForProvider.Username))
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	// Membership has no updatable fields besides the (immutable on spec)
	// team and username pair. Reaching Update means Observe found drift,
	// most likely re-add after manual deletion.
	cr, ok := mg.(*v1alpha1.Membership)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotMembership)
	}
	if cr.Spec.ForProvider.Username == nil || *cr.Spec.ForProvider.Username == "" {
		return managed.ExternalUpdate{}, errors.New(errMissingUsername)
	}
	id, err := e.resolveTeamID(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if err := e.api.AddMember(ctx, id, *cr.Spec.ForProvider.Username); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errAddMember)
	}
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Membership)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotMembership)
	}
	if cr.Spec.ForProvider.Username == nil || *cr.Spec.ForProvider.Username == "" {
		// Nothing actionable to remove.
		return managed.ExternalDelete{}, nil
	}
	id, err := e.resolveTeamID(ctx, cr)
	if err != nil {
		// Team gone (e.g. someone deleted it out of band) → membership is
		// trivially gone too. Other resolve errors (auth, network) bubble up.
		if errors.Is(err, ErrTeamNotFound) {
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, err
	}
	if err := e.api.RemoveMember(ctx, id, *cr.Spec.ForProvider.Username); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errRemoveMember)
	}
	return managed.ExternalDelete{}, nil
}
