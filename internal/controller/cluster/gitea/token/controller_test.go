// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/cluster/gitea/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

type fakeTokenAPI struct {
	listResp   []clients.TokenResource
	createResp *clients.TokenCreateResponse
	listCalls  int
	createArgs []struct {
		name   string
		scopes []string
	}
	deleteCalls []int64
}

func (f *fakeTokenAPI) List(ctx context.Context, username string) ([]clients.TokenResource, error) {
	f.listCalls++
	return f.listResp, nil
}

func (f *fakeTokenAPI) Create(ctx context.Context, username, name string, scopes []string) (*clients.TokenCreateResponse, error) {
	f.createArgs = append(f.createArgs, struct {
		name   string
		scopes []string
	}{name, scopes})
	return f.createResp, nil
}

func (f *fakeTokenAPI) Delete(ctx context.Context, username string, id int64) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return nil
}

func ptrStr(s string) *string { return &s }

func newCR() *v1alpha1.Token {
	return &v1alpha1.Token{
		ObjectMeta: metav1.ObjectMeta{Name: "hermes-agent-ci-bot-pat"},
		Spec: v1alpha1.TokenSpec{
			ForProvider: v1alpha1.TokenParameters{
				Name:   ptrStr("hermes-agent-ci-bot-pat"),
				Scopes: []string{"read:user", "write:package", "write:repository"},
			},
		},
	}
}

func TestObserve_UpToDate_WhenScopesMatch(t *testing.T) {
	api := &fakeTokenAPI{listResp: []clients.TokenResource{{ID: 42, Name: "hermes-agent-ci-bot-pat", Scopes: []string{"write:package", "read:user", "write:repository"}}}}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	got, err := e.Observe(context.Background(), newCR())
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !got.ResourceExists || !got.ResourceUpToDate {
		t.Fatalf("expected exists+upToDate, got %+v", got)
	}
}

func TestObserve_OutOfDate_WhenScopesDiffer(t *testing.T) {
	api := &fakeTokenAPI{listResp: []clients.TokenResource{{ID: 42, Name: "hermes-agent-ci-bot-pat", Scopes: []string{"write:package", "write:repository"}}}}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	got, _ := e.Observe(context.Background(), newCR())
	if !got.ResourceExists || got.ResourceUpToDate {
		t.Fatalf("expected exists+notUpToDate, got %+v", got)
	}
}

func TestObserve_NotExists_WhenNotInList(t *testing.T) {
	api := &fakeTokenAPI{listResp: nil}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	got, _ := e.Observe(context.Background(), newCR())
	if got.ResourceExists {
		t.Fatalf("expected ResourceExists=false")
	}
}

func TestCreate_PostsAndCapturesValue(t *testing.T) {
	api := &fakeTokenAPI{createResp: &clients.TokenCreateResponse{ID: 99, Name: "hermes-agent-ci-bot-pat", Scopes: []string{"read:user", "write:package", "write:repository"}, Sha1: "abcdef0123456789"}}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	cr := newCR()
	creation, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if string(creation.ConnectionDetails["token"]) != "abcdef0123456789" {
		t.Fatalf("token value not captured in ConnectionDetails")
	}
	if cr.Status.AtProvider.ID == nil || *cr.Status.AtProvider.ID != "99" {
		t.Fatalf("status.atProvider.id not set")
	}
	if cr.Status.AtProvider.LastEight == nil || *cr.Status.AtProvider.LastEight != "23456789" {
		t.Fatalf("status.atProvider.lastEight = %v, want 23456789", cr.Status.AtProvider.LastEight)
	}
}

func TestUpdate_RotatesByDeleteThenCreate(t *testing.T) {
	api := &fakeTokenAPI{createResp: &clients.TokenCreateResponse{ID: 100, Name: "hermes-agent-ci-bot-pat", Scopes: []string{"read:user", "write:package", "write:repository", "write:issue"}, Sha1: "freshvalue"}}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	cr := newCR()
	oldID := "42"
	cr.Status.AtProvider.ID = &oldID
	cr.Spec.ForProvider.Scopes = append(cr.Spec.ForProvider.Scopes, "write:issue")
	upd, err := e.Update(context.Background(), cr)
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != 42 {
		t.Fatalf("expected DELETE of id=42, got %v", api.deleteCalls)
	}
	if len(api.createArgs) != 1 {
		t.Fatalf("expected one Create call, got %d", len(api.createArgs))
	}
	if string(upd.ConnectionDetails["token"]) != "freshvalue" {
		t.Fatalf("rotated token value not in ConnectionDetails")
	}
	if *cr.Status.AtProvider.ID != "100" {
		t.Fatalf("new id not recorded")
	}
	if cr.Status.AtProvider.RotatedAt == nil {
		t.Fatalf("rotatedAt not recorded")
	}
}

func TestDelete_CallsAPI(t *testing.T) {
	api := &fakeTokenAPI{}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	cr := newCR()
	id := "42"
	cr.Status.AtProvider.ID = &id
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != 42 {
		t.Fatalf("unexpected Delete calls: %v", api.deleteCalls)
	}
}

func TestDelete_NoOpWhenNoID(t *testing.T) {
	api := &fakeTokenAPI{}
	e := &external{api: api, username: "hermes-agent-ci-bot"}
	if _, err := e.Delete(context.Background(), newCR()); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.deleteCalls) != 0 {
		t.Fatalf("expected no Delete calls without atProvider.id")
	}
}
