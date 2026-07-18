// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"context"
	"strconv"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/namespaced/gitea/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

const (
	errNotToken      = "managed resource is not a Token"
	errResolveCreds  = "cannot resolve gitea credentials"
	errMissingName   = "spec.forProvider.name is required"
	errMissingScopes = "spec.forProvider.scopes is required"
	errMissingUser   = "provider config credentials must contain a username (basic auth required)"
	errList          = "cannot list tokens from gitea"
	errCreate        = "cannot create token in gitea"
	errDelete        = "cannot delete token from gitea"
)

// Setup wires the controller for namespaced Token resources.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.Token_GroupVersionKind.String())
	r := managed.NewReconciler(mgr,
		xpresource.ManagedKind(v1alpha1.Token_GroupVersionKind),
		managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
	)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.Token{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated registers Setup behind the controller-engine gate.
func SetupGated(mgr ctrl.Manager, o tjcontroller.Options) error {
	o.Options.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "unable to setup reconciler", "gvk", v1alpha1.Token_GroupVersionKind.String())
		}
	}, v1alpha1.Token_GroupVersionKind)
	return nil
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Token)
	if !ok {
		return nil, errors.New(errNotToken)
	}
	creds, err := clients.ResolveGiteaCreds(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errResolveCreds)
	}
	if creds.Username == "" {
		return nil, errors.New(errMissingUser)
	}
	return &external{api: clients.NewGiteaTokenClient(creds), username: creds.Username}, nil
}

type external struct {
	api      clients.TokenAPI
	username string
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func tokenName(cr *v1alpha1.Token) string {
	if cr.Spec.ForProvider.Name == nil {
		return ""
	}
	return *cr.Spec.ForProvider.Name
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Token)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotToken)
	}
	name := tokenName(cr)
	if name == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	items, err := e.api.List(ctx, e.username)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errList)
	}
	var found *clients.TokenResource
	for i := range items {
		if items[i].Name == name {
			found = &items[i]
			break
		}
	}
	if found == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	idStr := strconv.FormatInt(found.ID, 10)
	cr.Status.AtProvider.ID = &idStr
	cr.Status.AtProvider.Scopes = found.Scopes
	meta.SetExternalName(cr, idStr)
	upToDate := clients.TokenScopesEqual(found.Scopes, cr.Spec.ForProvider.Scopes)
	cr.SetConditions(xpv1.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Token)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotToken)
	}
	name := tokenName(cr)
	if name == "" {
		return managed.ExternalCreation{}, errors.New(errMissingName)
	}
	if len(cr.Spec.ForProvider.Scopes) == 0 {
		return managed.ExternalCreation{}, errors.New(errMissingScopes)
	}
	resp, err := e.api.Create(ctx, e.username, name, clients.CanonicalTokenScopes(cr.Spec.ForProvider.Scopes))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}
	idStr := strconv.FormatInt(resp.ID, 10)
	cr.Status.AtProvider.ID = &idStr
	cr.Status.AtProvider.Scopes = resp.Scopes
	cr.Status.AtProvider.LastEight = stringPtr(lastEight(resp.Sha1))
	meta.SetExternalName(cr, idStr)
	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"token": []byte(resp.Sha1),
		},
	}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Token)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotToken)
	}
	name := tokenName(cr)
	if name == "" {
		return managed.ExternalUpdate{}, errors.New(errMissingName)
	}
	if len(cr.Spec.ForProvider.Scopes) == 0 {
		return managed.ExternalUpdate{}, errors.New(errMissingScopes)
	}
	if cr.Status.AtProvider.ID != nil && *cr.Status.AtProvider.ID != "" {
		id, perr := strconv.ParseInt(*cr.Status.AtProvider.ID, 10, 64)
		if perr == nil {
			if err := e.api.Delete(ctx, e.username, id); err != nil {
				return managed.ExternalUpdate{}, errors.Wrap(err, errDelete)
			}
		}
	}
	resp, err := e.api.Create(ctx, e.username, name, clients.CanonicalTokenScopes(cr.Spec.ForProvider.Scopes))
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errCreate)
	}
	idStr := strconv.FormatInt(resp.ID, 10)
	cr.Status.AtProvider.ID = &idStr
	cr.Status.AtProvider.Scopes = resp.Scopes
	cr.Status.AtProvider.LastEight = stringPtr(lastEight(resp.Sha1))
	cr.Status.AtProvider.RotatedAt = &metav1.Time{Time: time.Now()}
	meta.SetExternalName(cr, idStr)
	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{
			"token": []byte(resp.Sha1),
		},
	}, nil
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Token)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotToken)
	}
	if cr.Status.AtProvider.ID == nil || *cr.Status.AtProvider.ID == "" {
		return managed.ExternalDelete{}, nil
	}
	id, err := strconv.ParseInt(*cr.Status.AtProvider.ID, 10, 64)
	if err != nil {
		return managed.ExternalDelete{}, nil
	}
	if err := e.api.Delete(ctx, e.username, id); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

func lastEight(v string) string {
	if len(v) <= 8 {
		return v
	}
	return v[len(v)-8:]
}

func stringPtr(s string) *string { return &s }
