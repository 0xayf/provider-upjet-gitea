// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package membership

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/cluster/team/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

type fakeTeamAPI struct {
	getResp     *clients.TeamResource
	getErr      error
	memberResp  bool
	memberErr   error
	addErr      error
	rmErr       error
	addCalls    []memberArgs
	rmCalls     []memberArgs
	getCalls    []getArgs
	memberCalls []memberArgs
}
type memberArgs struct {
	id   int64
	user string
}
type getArgs struct {
	org, name string
}

func (f *fakeTeamAPI) Get(ctx context.Context, org, name string) (*clients.TeamResource, error) {
	f.getCalls = append(f.getCalls, getArgs{org, name})
	return f.getResp, f.getErr
}
func (f *fakeTeamAPI) GetByID(ctx context.Context, id int64) (*clients.TeamResource, error) {
	return f.getResp, f.getErr
}
func (f *fakeTeamAPI) Create(ctx context.Context, org string, p clients.TeamParams) (*clients.TeamResource, error) {
	return nil, nil
}
func (f *fakeTeamAPI) Update(ctx context.Context, id int64, p clients.TeamParams) (*clients.TeamResource, error) {
	return nil, nil
}
func (f *fakeTeamAPI) Delete(ctx context.Context, id int64) error                          { return nil }
func (f *fakeTeamAPI) AddRepository(ctx context.Context, id int64, repo string) error      { return nil }
func (f *fakeTeamAPI) RemoveRepository(ctx context.Context, id int64, repo string) error   { return nil }
func (f *fakeTeamAPI) ListRepositories(ctx context.Context, id int64) ([]string, error)    { return nil, nil }
func (f *fakeTeamAPI) IsMember(ctx context.Context, id int64, username string) (bool, error) {
	f.memberCalls = append(f.memberCalls, memberArgs{id, username})
	return f.memberResp, f.memberErr
}
func (f *fakeTeamAPI) AddMember(ctx context.Context, id int64, username string) error {
	f.addCalls = append(f.addCalls, memberArgs{id, username})
	return f.addErr
}
func (f *fakeTeamAPI) RemoveMember(ctx context.Context, id int64, username string) error {
	f.rmCalls = append(f.rmCalls, memberArgs{id, username})
	return f.rmErr
}

func ptrStr(s string) *string { return &s }

func newMembership(team *v1alpha1.TeamRef, username string) *v1alpha1.Membership {
	return &v1alpha1.Membership{
		ObjectMeta: metav1.ObjectMeta{Name: "m"},
		Spec: v1alpha1.MembershipSpec{
			ForProvider: v1alpha1.MembershipParameters{
				Team:     team,
				Username: ptrStr(username),
			},
		},
	}
}

func TestObserveMissingTeam(t *testing.T) {
	e := &external{api: &fakeTeamAPI{}}
	cr := newMembership(nil, "alice")
	if _, err := e.Observe(context.Background(), cr); err == nil {
		t.Fatalf("expected error for missing team ref")
	}
}

func TestObserveLegacyTeamID(t *testing.T) {
	// upjet-shape spec: only forProvider.teamId set, no team{} ref. The
	// resolver should use the numeric ID directly without an API lookup.
	api := &fakeTeamAPI{memberResp: true}
	e := &external{api: api}
	cr := &v1alpha1.Membership{
		ObjectMeta: metav1.ObjectMeta{Name: "m"},
		Spec: v1alpha1.MembershipSpec{
			ForProvider: v1alpha1.MembershipParameters{
				TeamID:   ptrFloat64(42),
				Username: ptrStr("alice"),
			},
		},
	}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatalf("expected resource exists for legacy teamId path")
	}
	if len(api.getCalls) != 0 {
		t.Fatalf("legacy teamId path should not call Get(org,name); got %v", api.getCalls)
	}
	if len(api.memberCalls) != 1 || api.memberCalls[0].id != 42 {
		t.Fatalf("expected IsMember(42, alice); got %v", api.memberCalls)
	}
}

func TestObserveTeamRefWinsOverLegacyTeamID(t *testing.T) {
	// Both set: spec.team {org,name} should win.
	api := &fakeTeamAPI{
		getResp:    &clients.TeamResource{ID: 99, Name: "h"},
		memberResp: true,
	}
	e := &external{api: api}
	cr := &v1alpha1.Membership{
		ObjectMeta: metav1.ObjectMeta{Name: "m"},
		Spec: v1alpha1.MembershipSpec{
			ForProvider: v1alpha1.MembershipParameters{
				Team:     &v1alpha1.TeamRef{Org: "apps", Name: "h"},
				TeamID:   ptrFloat64(42), // ignored
				Username: ptrStr("alice"),
			},
		},
	}
	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if len(api.memberCalls) != 1 || api.memberCalls[0].id != 99 {
		t.Fatalf("expected IsMember(99, ...) using team-ref-resolved ID; got %v", api.memberCalls)
	}
}

func ptrFloat64(f float64) *float64 { return &f }

func TestObserveMissingUsername(t *testing.T) {
	e := &external{api: &fakeTeamAPI{}}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "")
	if _, err := e.Observe(context.Background(), cr); err == nil {
		t.Fatalf("expected error for empty username")
	}
}

func TestObserveTeamNotFoundFailsResolve(t *testing.T) {
	api := &fakeTeamAPI{getResp: nil}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "missing"}, "alice")
	_, err := e.Observe(context.Background(), cr)
	if err == nil {
		t.Fatalf("expected resolve error")
	}
}

func TestObserveMemberPresent(t *testing.T) {
	api := &fakeTeamAPI{
		getResp:    &clients.TeamResource{ID: 5, Name: "h"},
		memberResp: true,
	}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !obs.ResourceExists || !obs.ResourceUpToDate {
		t.Fatalf("expected exists+upToDate, got %+v", obs)
	}
	if cr.Status.AtProvider.TeamID == nil || *cr.Status.AtProvider.TeamID != 5 {
		t.Fatalf("status.atProvider.teamId not cached")
	}
}

func TestObserveMemberAbsent(t *testing.T) {
	api := &fakeTeamAPI{
		getResp:    &clients.TeamResource{ID: 5, Name: "h"},
		memberResp: false,
	}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")

	obs, _ := e.Observe(context.Background(), cr)
	if obs.ResourceExists {
		t.Fatalf("expected ResourceExists=false when user not member")
	}
}

func TestCreateAddsMember(t *testing.T) {
	api := &fakeTeamAPI{getResp: &clients.TeamResource{ID: 5, Name: "h"}}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(api.addCalls) != 1 || api.addCalls[0].id != 5 || api.addCalls[0].user != "alice" {
		t.Fatalf("unexpected AddMember calls: %+v", api.addCalls)
	}
}

func TestCreateUsesCachedTeamID(t *testing.T) {
	api := &fakeTeamAPI{} // Get not stubbed — should not be called
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")
	id := int64(42)
	cr.Status.AtProvider.TeamID = &id

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(api.getCalls) != 0 {
		t.Fatalf("Get should not be called when team ID cached, got %v", api.getCalls)
	}
	if len(api.addCalls) != 1 || api.addCalls[0].id != 42 {
		t.Fatalf("unexpected AddMember calls: %+v", api.addCalls)
	}
}

func TestUpdateReAddsMember(t *testing.T) {
	api := &fakeTeamAPI{getResp: &clients.TeamResource{ID: 5, Name: "h"}}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")

	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if len(api.addCalls) != 1 {
		t.Fatalf("expected one AddMember call (re-add), got %d", len(api.addCalls))
	}
}

func TestDeleteRemovesMember(t *testing.T) {
	api := &fakeTeamAPI{getResp: &clients.TeamResource{ID: 5, Name: "h"}}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.rmCalls) != 1 || api.rmCalls[0].id != 5 || api.rmCalls[0].user != "alice" {
		t.Fatalf("unexpected RemoveMember calls: %+v", api.rmCalls)
	}
}

func TestDeleteNoOpWhenUsernameEmpty(t *testing.T) {
	api := &fakeTeamAPI{}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "")

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.rmCalls) != 0 {
		t.Fatalf("expected zero RemoveMember calls when username empty")
	}
}

func TestDeletePropagatesResolveError(t *testing.T) {
	api := &fakeTeamAPI{getErr: errors.New("network")}
	e := &external{api: api}
	cr := newMembership(&v1alpha1.TeamRef{Org: "apps", Name: "h"}, "alice")

	_, err := e.Delete(context.Background(), cr)
	if err == nil {
		t.Fatalf("expected error from Delete when team resolve fails")
	}
}
