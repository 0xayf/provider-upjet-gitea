// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package useractionssecret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
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
	errNotUserActionsSecret = "managed resource is not a UserActionsSecret"
	errResolveCreds         = "cannot resolve gitea credentials"
	errReadValueSecret      = "cannot read value secret referenced by spec.forProvider.secretValueSecretRef"
	errMissingValueKey      = "value secret has no data at key referenced by spec.forProvider.secretValueSecretRef"
	errGet                  = "cannot get actions secret from gitea"
	errPut                  = "cannot put actions secret to gitea"
	errDelete               = "cannot delete actions secret from gitea"
)

// Setup wires the controller for namespaced UserActionsSecret resources.
func Setup(mgr ctrl.Manager, o tjcontroller.Options) error {
	name := managed.ControllerName(v1alpha1.UserActionsSecret_GroupVersionKind.String())

	r := managed.NewReconciler(mgr,
		xpresource.ManagedKind(v1alpha1.UserActionsSecret_GroupVersionKind),
		managed.WithExternalConnecter(&connector{kube: mgr.GetClient()}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.UserActionsSecret{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// SetupGated registers Setup behind the controller-engine gate.
func SetupGated(mgr ctrl.Manager, o tjcontroller.Options) error {
	o.Options.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "unable to setup reconciler", "gvk", v1alpha1.UserActionsSecret_GroupVersionKind.String())
		}
	}, v1alpha1.UserActionsSecret_GroupVersionKind)
	return nil
}

type connector struct {
	kube client.Client
}

func (c *connector) Connect(ctx context.Context, mg xpresource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.UserActionsSecret)
	if !ok {
		return nil, errors.New(errNotUserActionsSecret)
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

func scopeFor(_ *v1alpha1.UserActionsSecret) clients.ActionsSecretScope {
	return clients.ActionsSecretScope{User: true}
}

func secretName(cr *v1alpha1.UserActionsSecret) string {
	if cr.Spec.ForProvider.SecretName == nil {
		return ""
	}
	return *cr.Spec.ForProvider.SecretName
}

func (e *external) Observe(ctx context.Context, mg xpresource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.UserActionsSecret)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotUserActionsSecret)
	}
	name := secretName(cr)
	if name == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	// Gitea exposes no list/get for /user/actions/secrets, so we cannot
	// query existence directly. Use the reconciler-managed
	// crossplane.io/external-create-succeeded annotation as the marker.
	// It is set on the resource by Crossplane after a successful Create
	// and persists independently of any status mutations.
	if _, succeeded := cr.GetAnnotations()[annotationCreateSucceeded]; !succeeded {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	currentHash := hashValue(value)
	upToDate := cr.Status.AtProvider.ValueHash != nil && *cr.Status.AtProvider.ValueHash == currentHash

	// Persist atProvider via Observe (mutations from Observe are reliably
	// written; mutations from Create can be dropped by status update
	// races). Existed mirrors the annotation for human-readable status;
	// ValueHash is preserved across reconciles so Update can replace it
	// when the source rotates.
	previousHash := cr.Status.AtProvider.ValueHash
	cr.Status.AtProvider.Existed = boolPtr(true)
	cr.Status.AtProvider.ValueHash = previousHash
	// The Crossplane managed reconciler does not auto-transition Ready
	// from Creating to Available; the external client must mark it.
	cr.SetConditions(xpv1.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

// annotationCreateSucceeded is the annotation Crossplane's managed
// reconciler sets on a resource after a successful Create call.
const annotationCreateSucceeded = "crossplane.io/external-create-succeeded"

func (e *external) Create(ctx context.Context, mg xpresource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.UserActionsSecret)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotUserActionsSecret)
	}
	value, err := readLocalValueSecret(ctx, e.kube, cr.Namespace, cr.Spec.ForProvider.SecretValueSecretRef.Name, cr.Spec.ForProvider.SecretValueSecretRef.Key)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	if err := e.api.Put(ctx, scopeFor(cr), secretName(cr), value); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errPut)
	}
	cr.Status.AtProvider.Existed = boolPtr(true)
	cr.Status.AtProvider.ValueHash = stringPtr(hashValue(value))
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg xpresource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.UserActionsSecret)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotUserActionsSecret)
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

// hashValue returns the hex SHA-256 of v.
func hashValue(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// boolPtr is a tiny helper for taking the address of a literal bool.
func boolPtr(b bool) *bool { return &b }

func (e *external) Delete(ctx context.Context, mg xpresource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.UserActionsSecret)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotUserActionsSecret)
	}
	if err := e.api.Delete(ctx, scopeFor(cr), secretName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

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
