// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package orgactionssecret

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

func newCR() *v1alpha1.OrgActionsSecret {
	return &v1alpha1.OrgActionsSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.OrgActionsSecretSpec{
			ForProvider: v1alpha1.OrgActionsSecretParameters{
				Org:        ptrStr("apps"),
				SecretName: ptrStr("CI_BOT_TOKEN"),
				SecretValueSecretRef: xpv1.SecretKeySelector{
					SecretReference: xpv1.SecretReference{Name: "source-secret", Namespace: "src-ns"},
					Key:             "token",
				},
			},
		},
	}
}

func TestObserve_Exists_AndUpToDate_WhenHashMatches(t *testing.T) {
	cr := newCR()
	cr.Status.AtProvider.ValueHash = stringPtrFromHashOf("the-value")
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "src-ns"},
		Data:       map[string][]byte{"token": []byte("the-value")},
	}
	api := &fakeAPI{getResp: &clients.ActionsSecretResource{Name: "CI_BOT_TOKEN", CreatedAt: "2026-01-01T00:00:00Z"}}
	e := &external{kube: newKube(t, src).Build(), api: api}
	got, _ := e.Observe(context.Background(), cr)
	if !got.ResourceExists || !got.ResourceUpToDate {
		t.Fatalf("expected exists+uptodate, got: %+v", got)
	}
}

func TestObserve_NotUpToDate_WhenHashDiffers(t *testing.T) {
	cr := newCR()
	cr.Status.AtProvider.ValueHash = stringPtrFromHashOf("original")
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "src-ns"},
		Data:       map[string][]byte{"token": []byte("rotated")},
	}
	api := &fakeAPI{getResp: &clients.ActionsSecretResource{Name: "CI_BOT_TOKEN"}}
	e := &external{kube: newKube(t, src).Build(), api: api}
	got, _ := e.Observe(context.Background(), cr)
	if !got.ResourceExists || got.ResourceUpToDate {
		t.Fatalf("expected exists but not up-to-date, got: %+v", got)
	}
}

func TestCreate_ReadsCrossNamespaceSource(t *testing.T) {
	cr := newCR()
	src := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "src-ns"},
		Data:       map[string][]byte{"token": []byte("xn-value")},
	}
	api := &fakeAPI{}
	e := &external{kube: newKube(t, src).Build(), api: api}
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if api.putCalls[0].value != "xn-value" {
		t.Fatalf("expected cross-namespace value passed through, got: %s", api.putCalls[0].value)
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

func TestDelete_CallsAPI(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{}
	e := &external{api: api}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if len(api.deleteCalls) != 1 {
		t.Fatalf("expected 1 delete, got %d", len(api.deleteCalls))
	}
}
