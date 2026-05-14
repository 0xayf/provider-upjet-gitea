// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package actionssecret

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

func hashOf(s string) *string {
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

func newCR() *v1alpha1.ActionsSecret {
	return &v1alpha1.ActionsSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.ActionsSecretSpec{
			ForProvider: v1alpha1.ActionsSecretParameters{
				RepositoryOwner: ptrStr("apps"),
				Repository:      ptrStr("hermes-agent"),
				SecretName:      ptrStr("CI_BOT_TOKEN"),
				SecretValueSecretRef: xpv1.SecretKeySelector{
					SecretReference: xpv1.SecretReference{Name: "source-secret", Namespace: "src-ns"},
					Key:             "token",
				},
			},
		},
	}
}

func newSourceSecret(value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "source-secret", Namespace: "src-ns"},
		Data:       map[string][]byte{"token": []byte(value)},
	}
}

func TestObserve_UpToDate_WhenHashMatches(t *testing.T) {
	cr := newCR()
	cr.Status.AtProvider.ValueHash = hashOf("v1")
	api := &fakeAPI{getResp: &clients.ActionsSecretResource{Name: "CI_BOT_TOKEN"}}
	e := &external{kube: newKube(t, newSourceSecret("v1")).Build(), api: api}
	got, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !got.ResourceExists || !got.ResourceUpToDate {
		t.Fatalf("expected exists+upToDate, got %+v", got)
	}
}

func TestObserve_OutOfDate_WhenHashDiffers(t *testing.T) {
	cr := newCR()
	cr.Status.AtProvider.ValueHash = hashOf("old")
	api := &fakeAPI{getResp: &clients.ActionsSecretResource{Name: "CI_BOT_TOKEN"}}
	e := &external{kube: newKube(t, newSourceSecret("rotated")).Build(), api: api}
	got, _ := e.Observe(context.Background(), cr)
	if !got.ResourceExists || got.ResourceUpToDate {
		t.Fatalf("expected exists+notUpToDate, got %+v", got)
	}
}

func TestObserve_NotExists_WhenGiteaReturnsNil(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{getResp: nil}
	e := &external{kube: newKube(t, newSourceSecret("v1")).Build(), api: api}
	got, _ := e.Observe(context.Background(), cr)
	if got.ResourceExists {
		t.Fatalf("expected resource exists=false when Gitea returns nil")
	}
}

func TestCreate_PutsToGitea_AndRecordsHash(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{}
	e := &external{kube: newKube(t, newSourceSecret("new-value")).Build(), api: api}
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(api.putCalls) != 1 || api.putCalls[0].value != "new-value" {
		t.Fatalf("unexpected Put calls: %+v", api.putCalls)
	}
	if cr.Status.AtProvider.ValueHash == nil || *cr.Status.AtProvider.ValueHash != *hashOf("new-value") {
		t.Fatalf("valueHash not recorded")
	}
}

func TestUpdate_PutsRotatedValue_AndRefreshesHash(t *testing.T) {
	cr := newCR()
	cr.Status.AtProvider.ValueHash = hashOf("old")
	api := &fakeAPI{}
	e := &external{kube: newKube(t, newSourceSecret("new")).Build(), api: api}
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if len(api.putCalls) != 1 || api.putCalls[0].value != "new" {
		t.Fatalf("expected single Put with rotated value, got %+v", api.putCalls)
	}
	if *cr.Status.AtProvider.ValueHash != *hashOf("new") {
		t.Fatalf("valueHash not refreshed")
	}
}

func TestDelete_CallsAPI(t *testing.T) {
	cr := newCR()
	api := &fakeAPI{}
	e := &external{kube: newKube(t).Build(), api: api}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0].name != "CI_BOT_TOKEN" {
		t.Fatalf("unexpected Delete calls: %+v", api.deleteCalls)
	}
}
