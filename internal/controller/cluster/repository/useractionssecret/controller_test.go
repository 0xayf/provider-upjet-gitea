// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package useractionssecret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/cluster/repository/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

func stringPtrFromHashOf(s string) *string {
	sum := sha256.Sum256([]byte(s))
	h := hex.EncodeToString(sum[:])
	return &h
}

func boolPtrTrue() *bool { b := true; return &b }

type fakeAPI struct {
	getResp     *clients.ActionsSecretResource
	putCalls    []putArgs
	deleteCalls []deleteArgs
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
	return f.getResp, nil
}

func (f *fakeAPI) Put(ctx context.Context, scope clients.ActionsSecretScope, name, value string) error {
	f.putCalls = append(f.putCalls, putArgs{scope, name, value})
	return nil
}

func (f *fakeAPI) Delete(ctx context.Context, scope clients.ActionsSecretScope, name string) error {
	f.deleteCalls = append(f.deleteCalls, deleteArgs{scope, name})
	return nil
}

func newKube(t *testing.T, objs ...runtime.Object) *fake.ClientBuilder {
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

func ptrStr(s string) *string { return &s }

func newCR() *v1alpha1.UserActionsSecret {
	return &v1alpha1.UserActionsSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.UserActionsSecretSpec{
			ForProvider: v1alpha1.UserActionsSecretParameters{
				SecretName: ptrStr("MY_TOKEN"),
				SecretValueSecretRef: xpv1.SecretKeySelector{
					SecretReference: xpv1.SecretReference{Name: "source-secret", Namespace: "src-ns"},
					Key:             "token",
				},
			},
		},
	}
}

func TestCreate_UsesUserScope_AndSetsExistedAndHash(t *testing.T) {
	cr := newCR()
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "src-ns"},
		Data:       map[string][]byte{"token": []byte("v")},
	}
	api := &fakeAPI{}
	e := &external{kube: newKube(t, src).Build(), api: api}
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !api.putCalls[0].scope.User {
		t.Fatalf("expected scope.User=true")
	}
	if cr.Status.AtProvider.Existed == nil || !*cr.Status.AtProvider.Existed {
		t.Fatalf("expected Existed=true after Create")
	}
	expectedHash := *stringPtrFromHashOf("v")
	if cr.Status.AtProvider.ValueHash == nil || *cr.Status.AtProvider.ValueHash != expectedHash {
		t.Fatalf("expected ValueHash to record value SHA, got: %v", cr.Status.AtProvider.ValueHash)
	}
}

func TestObserve_NotExists_BeforeCreate(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{}
	e := &external{api: api}
	got, _ := e.Observe(context.Background(), cr)
	if got.ResourceExists {
		t.Fatalf("expected ResourceExists=false before Create")
	}
}

func TestObserve_Exists_AndUpToDate_AfterCreate(t *testing.T) {
	cr := newCR()
	cr.Status.AtProvider.Existed = boolPtrTrue()
	cr.Status.AtProvider.ValueHash = stringPtrFromHashOf("v")
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "src-ns"},
		Data:       map[string][]byte{"token": []byte("v")},
	}
	api := &fakeAPI{}
	e := &external{kube: newKube(t, src).Build(), api: api}
	got, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("observe failed: %v", err)
	}
	if !got.ResourceExists || !got.ResourceUpToDate {
		t.Fatalf("expected exists+uptodate, got: %+v", got)
	}
}

func TestCreate_ErrorWhenNamespaceMissing(t *testing.T) {
	cr := newCR()
	cr.Spec.ForProvider.SecretValueSecretRef.Namespace = ""
	api := &fakeAPI{}
	e := &external{kube: newKube(t).Build(), api: api}
	if _, err := e.Create(context.Background(), cr); err == nil {
		t.Fatal("expected error when secretValueSecretRef.namespace is empty for cluster scope")
	}
}
