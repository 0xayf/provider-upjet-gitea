// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package actionssecret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	tjcontroller "github.com/crossplane/upjet/v2/pkg/controller"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/namespaced/repository/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

const (
	errNotActionsSecret = "managed resource is not an ActionsSecret"
	errResolveCreds     = "cannot resolve gitea credentials"
	errReadValueSecret  = "cannot read value secret referenced by spec.forProvider.secretValueSecretRef"
	errMissingValueKey  = "value secret has no data at key referenced by spec.forProvider.secretValueSecretRef"
	errGet              = "cannot get actions secret from gitea"
	errPut              = "cannot put actions secret to gitea"
	errDelete           = "cannot delete actions secret from gitea"
)

// Setup wires the controller for namespaced ActionsSecret resources.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.ActionsSecret_GroupVersionKind.String())
	r := managed.NewReconciler(mgr,
		xpresource.ManagedKind(v1alpha1.ActionsSecret_GroupVersionKind),
		managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
	)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.ActionsSecret{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated registers Setup behind the controller-engine gate.
func SetupGated(mgr ctrl.Manager, o tjcontroller.Options) error {
	o.Options.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "unable to setup reconciler", "gvk", v1alpha1.ActionsSecret_GroupVersionKind.String())
		}
	}, v1alpha1.ActionsSecret_GroupVersionKind)
	return nil
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.ActionsSecret)
	if !ok {
		return nil, errors.New(errNotActionsSecret)
	}
	creds, err := clients.ResolveGiteaCreds(ctx, c.kube, cr)
	if err != nil {
		return nil, errors.Wrap(err, errResolveCreds)
	}
	return &external{kube: c.kube, api: clients.NewGiteaActionsSecretClient(creds)}, nil
}

type external struct {
	kube client.Client
	api  clients.ActionsSecretAPI
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func scopeFor(cr *v1alpha1.ActionsSecret) (clients.ActionsSecretScope, error) {
	if cr.Spec.ForProvider.RepositoryOwner == nil || *cr.Spec.ForProvider.RepositoryOwner == "" {
		return clients.ActionsSecretScope{}, errors.New("spec.forProvider.repositoryOwner is required")
	}
	if cr.Spec.ForProvider.Repository == nil || *cr.Spec.ForProvider.Repository == "" {
		return clients.ActionsSecretScope{}, errors.New("spec.forProvider.repository is required")
	}
	return clients.ActionsSecretScope{
		Repository: &clients.RepoTarget{
			Owner: *cr.Spec.ForProvider.RepositoryOwner,
			Name:  *cr.Spec.ForProvider.Repository,
		},
	}, nil
}

func secretName(cr *v1alpha1.ActionsSecret) string {
	if cr.Spec.ForProvider.SecretName == nil {
		return ""
	}
	return *cr.Spec.ForProvider.SecretName
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.ActionsSecret)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotActionsSecret)
	}
	name := secretName(cr)
	if name == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	scope, err := scopeFor(cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	got, err := e.api.Get(ctx, scope, name)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGet)
	}
	if got == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	currentHash := hashValue(value)
	upToDate := cr.Status.AtProvider.ValueHash != nil && *cr.Status.AtProvider.ValueHash == currentHash

	cr.Status.AtProvider.CreatedAt = stringPtr(got.CreatedAt)
	meta.SetExternalName(cr, externalNameFor(cr))
	cr.SetConditions(xpv1.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.ActionsSecret)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotActionsSecret)
	}
	scope, err := scopeFor(cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if err := e.api.Put(ctx, scope, secretName(cr), value); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errPut)
	}
	cr.Status.AtProvider.ValueHash = stringPtr(hashValue(value))
	meta.SetExternalName(cr, externalNameFor(cr))
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.ActionsSecret)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotActionsSecret)
	}
	scope, err := scopeFor(cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if err := e.api.Put(ctx, scope, secretName(cr), value); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errPut)
	}
	cr.Status.AtProvider.ValueHash = stringPtr(hashValue(value))
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.ActionsSecret)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotActionsSecret)
	}
	scope, err := scopeFor(cr)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	if err := e.api.Delete(ctx, scope, secretName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

func externalNameFor(cr *v1alpha1.ActionsSecret) string {
	owner := ""
	if cr.Spec.ForProvider.RepositoryOwner != nil {
		owner = *cr.Spec.ForProvider.RepositoryOwner
	}
	repo := ""
	if cr.Spec.ForProvider.Repository != nil {
		repo = *cr.Spec.ForProvider.Repository
	}
	return fmt.Sprintf("%s:%s:%s", owner, repo, secretName(cr))
}

func hashValue(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func readLocalValueSecret(ctx context.Context, kube client.Client, namespace, name, key string) (string, error) {
	if namespace == "" || name == "" || key == "" {
		return "", errors.New(errReadValueSecret)
	}
	s := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s); err != nil {
		return "", errors.Wrap(err, errReadValueSecret)
	}
	v, ok := s.Data[key]
	if !ok || len(v) == 0 {
		return "", errors.New(errMissingValueKey)
	}
	return string(v), nil
}

func stringPtr(s string) *string { return &s }
