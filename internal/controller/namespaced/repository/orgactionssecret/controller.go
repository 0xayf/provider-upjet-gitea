// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package orgactionssecret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
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
	errNotOrgActionsSecret = "managed resource is not an OrgActionsSecret"
	errResolveCreds        = "cannot resolve gitea credentials"
	errReadValueSecret     = "cannot read value secret referenced by spec.forProvider.secretValueSecretRef"
	errMissingValueKey     = "value secret has no data at key referenced by spec.forProvider.secretValueSecretRef"
	errGet                 = "cannot get actions secret from gitea"
	errPut                 = "cannot put actions secret to gitea"
	errDelete              = "cannot delete actions secret from gitea"
)

// Setup wires the controller for namespaced OrgActionsSecret resources.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.OrgActionsSecret_GroupVersionKind.String())

	r := managed.NewReconciler(mgr,
		xpresource.ManagedKind(v1alpha1.OrgActionsSecret_GroupVersionKind),
		managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.OrgActionsSecret{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated registers Setup behind the controller-engine gate.
func SetupGated(mgr ctrl.Manager, o tjcontroller.Options) error {
	o.Options.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "unable to setup reconciler", "gvk", v1alpha1.OrgActionsSecret_GroupVersionKind.String())
		}
	}, v1alpha1.OrgActionsSecret_GroupVersionKind)
	return nil
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.OrgActionsSecret)
	if !ok {
		return nil, errors.New(errNotOrgActionsSecret)
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

func scopeFor(cr *v1alpha1.OrgActionsSecret) clients.ActionsSecretScope {
	return clients.ActionsSecretScope{Organization: cr.Spec.ForProvider.Org}
}

func secretName(cr *v1alpha1.OrgActionsSecret) string {
	if cr.Spec.ForProvider.SecretName == nil {
		return ""
	}
	return *cr.Spec.ForProvider.SecretName
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.OrgActionsSecret)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotOrgActionsSecret)
	}
	name := secretName(cr)
	if name == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	got, err := e.api.Get(ctx, scopeFor(cr), name)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGet)
	}
	if got == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	// Hash the current source value to detect rotation. Gitea's API does not
	// expose the secret value for a direct comparison, so we compare the
	// current source's SHA-256 against the hash recorded when we last
	// pushed.
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	currentHash := hashValue(value)
	upToDate := cr.Status.AtProvider.ValueHash != nil && *cr.Status.AtProvider.ValueHash == currentHash

	previousHash := cr.Status.AtProvider.ValueHash
	cr.Status.AtProvider.CreatedAt = stringPtr(got.CreatedAt)
	cr.Status.AtProvider.ValueHash = previousHash // preserve until Create/Update writes a new one
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.OrgActionsSecret)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotOrgActionsSecret)
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if err := e.api.Put(ctx, scopeFor(cr), secretName(cr), value); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errPut)
	}
	cr.Status.AtProvider.ValueHash = stringPtr(hashValue(value))
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.OrgActionsSecret)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotOrgActionsSecret)
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if err := e.api.Put(ctx, scopeFor(cr), secretName(cr), value); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errPut)
	}
	cr.Status.AtProvider.ValueHash = stringPtr(hashValue(value))
	return managed.ExternalUpdate{}, nil
}

// hashValue returns the hex SHA-256 of v, used as a stable fingerprint of
// the secret value. The full hash (not a prefix) is stored so very-similar
// values are still distinguished.
func hashValue(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.OrgActionsSecret)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotOrgActionsSecret)
	}
	if err := e.api.Delete(ctx, scopeFor(cr), secretName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

// readLocalValueSecret resolves a same-namespace secret reference and returns
// the value at the requested key.
func readLocalValueSecret(ctx context.Context, kube client.Client, namespace, name, key string) (string, error) {
	if name == "" || key == "" {
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
