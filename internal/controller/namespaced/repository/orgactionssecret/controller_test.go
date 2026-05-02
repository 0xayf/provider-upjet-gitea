// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package orgactionssecret

import (
	"context"
	"errors"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/namespaced/repository/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

// fakeAPI captures Put/Delete arguments and returns canned Get responses.
type fakeAPI struct {
	get func(ctx context.Context, scope clients.ActionsSecretScope, name string) (*clients.ActionsSecretResource, error)

	putCalls    []putArgs
	deleteCalls []deleteArgs

	putErr    error
	deleteErr error
}

type putArgs struct {
	scope clients.ActionsSecretScope
	name  string
	value string
}

type deleteArgs struct {
	scope clients.ActionsSecretScope
	name  string
}

func (f *fakeAPI) Get(ctx context.Context, scope clients.ActionsSecretScope, name string) (*clients.ActionsSecretResource, error) {
	if f.get != nil {
		return f.get(ctx, scope, name)
	}
	return nil, nil
}

func (f *fakeAPI) Put(ctx context.Context, scope clients.ActionsSecretScope, name, value string) error {
	f.putCalls = append(f.putCalls, putArgs{scope: scope, name: name, value: value})
	return f.putErr
}

func (f *fakeAPI) Delete(ctx context.Context, scope clients.ActionsSecretScope, name string) error {
	f.deleteCalls = append(f.deleteCalls, deleteArgs{scope: scope, name: name})
	return f.deleteErr
}

func ptrStr(s string) *string { return &s }

func newFakeKube(t *testing.T, objs ...runtime.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := v1alpha1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatalf("add v1alpha1 scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

func newCR() *v1alpha1.OrgActionsSecret {
	return &v1alpha1.OrgActionsSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test-ns"},
		Spec: v1alpha1.OrgActionsSecretSpec{
			ForProvider: v1alpha1.OrgActionsSecretParameters{
				Org:        ptrStr("apps"),
				SecretName: ptrStr("CI_BOT_TOKEN"),
				SecretValueSecretRef: xpv1.LocalSecretKeySelector{
					LocalSecretReference: xpv1.LocalSecretReference{Name: "source-secret"},
					Key:                  "token",
				},
			},
		},
	}
}

func TestObserve_NotExists_WhenSecretMissing(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{get: func(_ context.Context, _ clients.ActionsSecretScope, _ string) (*clients.ActionsSecretResource, error) {
		return nil, nil
	}}
	e := &external{api: api}
	got, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	if got.ResourceExists {
		t.Fatalf("expected ResourceExists=false, got true")
	}
}

func TestObserve_Exists_WhenSecretFound(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{get: func(_ context.Context, _ clients.ActionsSecretScope, _ string) (*clients.ActionsSecretResource, error) {
		return &clients.ActionsSecretResource{Name: "CI_BOT_TOKEN", CreatedAt: "2026-01-01T00:00:00Z"}, nil
	}}
	e := &external{api: api}
	got, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	if !got.ResourceExists {
		t.Fatalf("expected ResourceExists=true")
	}
	if !got.ResourceUpToDate {
		t.Fatalf("expected ResourceUpToDate=true")
	}
	if cr.Status.AtProvider.CreatedAt == nil || *cr.Status.AtProvider.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("expected status.atProvider.createdAt to be propagated")
	}
}

func TestObserve_GetFailurePropagatesError(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{get: func(_ context.Context, _ clients.ActionsSecretScope, _ string) (*clients.ActionsSecretResource, error) {
		return nil, errors.New("boom")
	}}
	e := &external{api: api}
	if _, err := e.Observe(context.Background(), cr); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreate_ReadsValueAndPutsToCorrectScope(t *testing.T) {
	cr := newCR()
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"token": []byte("the-secret-value")},
	}
	api := &fakeAPI{}
	e := &external{kube: newFakeKube(t, src).Build(), api: api}

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if len(api.putCalls) != 1 {
		t.Fatalf("expected 1 put, got %d", len(api.putCalls))
	}
	got := api.putCalls[0]
	if got.value != "the-secret-value" {
		t.Fatalf("expected value passed through, got: %s", got.value)
	}
	if got.scope.Organization == nil || *got.scope.Organization != "apps" {
		t.Fatalf("expected scope.org=apps, got: %+v", got.scope)
	}
	if got.name != "CI_BOT_TOKEN" {
		t.Fatalf("expected name=CI_BOT_TOKEN, got: %s", got.name)
	}
}

func TestCreate_ErrorWhenSourceSecretMissing(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{}
	e := &external{kube: newFakeKube(t).Build(), api: api}

	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("expected error for missing source secret")
	}
	if len(api.putCalls) != 0 {
		t.Fatalf("expected no put when source missing, got %d", len(api.putCalls))
	}
}

func TestCreate_ErrorWhenSourceKeyMissing(t *testing.T) {
	cr := newCR()
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"different-key": []byte("v")},
	}
	api := &fakeAPI{}
	e := &external{kube: newFakeKube(t, src).Build(), api: api}

	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("expected error for missing key in source secret")
	}
}

func TestUpdate_ReissuesPut(t *testing.T) {
	cr := newCR()
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"token": []byte("rotated-value")},
	}
	api := &fakeAPI{}
	e := &external{kube: newFakeKube(t, src).Build(), api: api}

	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if len(api.putCalls) != 1 || api.putCalls[0].value != "rotated-value" {
		t.Fatalf("expected put with rotated value, got: %+v", api.putCalls)
	}
}

func TestDelete_CallsAPI(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{}
	e := &external{api: api}

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if len(api.deleteCalls) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(api.deleteCalls))
	}
	if api.deleteCalls[0].name != "CI_BOT_TOKEN" {
		t.Fatalf("unexpected delete name: %s", api.deleteCalls[0].name)
	}
}

func TestObserve_NoOpWhenSecretNameEmpty(t *testing.T) {
	cr := newCR()
	cr.Spec.ForProvider.SecretName = nil
	api := &fakeAPI{}
	e := &external{api: api}

	got, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	if got.ResourceExists {
		t.Fatalf("expected ResourceExists=false when name unset")
	}
}
