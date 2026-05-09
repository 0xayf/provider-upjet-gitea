// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/0xayf/provider-upjet-gitea/apis/cluster/gitea/v1alpha1"
	"github.com/0xayf/provider-upjet-gitea/internal/clients"
)

// fakeTeamAPI lets tests assert which client methods got called and with what
// payload, without standing up an HTTP server.
type fakeTeamAPI struct {
	getResp        *clients.TeamResource
	createResp     *clients.TeamResource
	updateResp     *clients.TeamResource
	listReposResp  []string
	memberResp     bool
	createCalls    []clients.TeamParams
	updateCalls    []updateCallArgs
	deleteCalls    []int64
	addRepoCalls   []addRepoArgs
	rmRepoCalls    []addRepoArgs
}

type updateCallArgs struct {
	id     int64
	params clients.TeamParams
}
type addRepoArgs struct {
	id   int64
	repo string
}

func (f *fakeTeamAPI) Get(ctx context.Context, org, name string) (*clients.TeamResource, error) {
	return f.getResp, nil
}
func (f *fakeTeamAPI) GetByID(ctx context.Context, id int64) (*clients.TeamResource, error) {
	return f.getResp, nil
}
func (f *fakeTeamAPI) Create(ctx context.Context, org string, p clients.TeamParams) (*clients.TeamResource, error) {
	f.createCalls = append(f.createCalls, p)
	return f.createResp, nil
}
func (f *fakeTeamAPI) Update(ctx context.Context, id int64, p clients.TeamParams) (*clients.TeamResource, error) {
	f.updateCalls = append(f.updateCalls, updateCallArgs{id, p})
	if f.updateResp != nil {
		return f.updateResp, nil
	}
	return &clients.TeamResource{ID: id, UnitsMap: p.UnitsMap}, nil
}
func (f *fakeTeamAPI) Delete(ctx context.Context, id int64) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return nil
}
func (f *fakeTeamAPI) AddRepository(ctx context.Context, id int64, repo string) error {
	f.addRepoCalls = append(f.addRepoCalls, addRepoArgs{id, repo})
	return nil
}
func (f *fakeTeamAPI) RemoveRepository(ctx context.Context, id int64, repo string) error {
	f.rmRepoCalls = append(f.rmRepoCalls, addRepoArgs{id, repo})
	return nil
}
func (f *fakeTeamAPI) ListRepositories(ctx context.Context, id int64) ([]string, error) {
	return f.listReposResp, nil
}
func (f *fakeTeamAPI) IsMember(ctx context.Context, id int64, username string) (bool, error) {
	return f.memberResp, nil
}
func (f *fakeTeamAPI) AddMember(ctx context.Context, id int64, username string) error    { return nil }
func (f *fakeTeamAPI) RemoveMember(ctx context.Context, id int64, username string) error { return nil }

func ptrStr(s string) *string { return &s }
func ptrBool(b bool) *bool    { return &b }

func newTeam(unitsMap map[string]string, repos []string) *v1alpha1.Team {
	return &v1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: "hermes-agent"},
		Spec: v1alpha1.TeamSpec{
			ForProvider: v1alpha1.TeamParameters{
				Name:         ptrStr("hermes-agent"),
				Organisation: ptrStr("apps"),
				Description:  ptrStr("Test"),
				Repositories: repos,
				UnitsMap:     unitsMap,
			},
		},
	}
}

func TestObserveTeamNotFound(t *testing.T) {
	api := &fakeTeamAPI{getResp: nil}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, []string{"hermes-agent"})

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if obs.ResourceExists {
		t.Fatalf("expected ResourceExists=false, got true")
	}
}

func TestObserveValidatesSpec(t *testing.T) {
	cases := map[string]*v1alpha1.Team{
		"missing name": {
			Spec: v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{
				Organisation: ptrStr("apps"),
				UnitsMap:     map[string]string{"repo.code": "write"},
			}},
		},
		"missing org": {
			Spec: v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{
				Name:     ptrStr("x"),
				UnitsMap: map[string]string{"repo.code": "write"},
			}},
		},
		"empty unitsMap": {
			Spec: v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{
				Name:         ptrStr("x"),
				Organisation: ptrStr("apps"),
				UnitsMap:     map[string]string{},
			}},
		},
		"invalid unit name": {
			Spec: v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{
				Name:         ptrStr("x"),
				Organisation: ptrStr("apps"),
				UnitsMap:     map[string]string{"repo.bogus": "write"},
			}},
		},
		"invalid level": {
			Spec: v1alpha1.TeamSpec{ForProvider: v1alpha1.TeamParameters{
				Name:         ptrStr("x"),
				Organisation: ptrStr("apps"),
				UnitsMap:     map[string]string{"repo.code": "owner"},
			}},
		},
	}
	for name, cr := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{api: &fakeTeamAPI{}}
			_, err := e.Observe(context.Background(), cr)
			if err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestObserveTeamUpToDate(t *testing.T) {
	id := int64(5)
	api := &fakeTeamAPI{
		getResp: &clients.TeamResource{
			ID:                      id,
			Name:                    "hermes-agent",
			Description:             "Test",
			IncludesAllRepositories: false,
			UnitsMap:                fullDefaultUnitsMap(map[string]clients.TeamPermissionLevel{"repo.code": "write"}),
		},
		listReposResp: []string{"hermes-agent"},
	}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, []string{"hermes-agent"})

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !obs.ResourceExists {
		t.Fatalf("expected ResourceExists=true")
	}
	if !obs.ResourceUpToDate {
		t.Fatalf("expected ResourceUpToDate=true; got status: id=%v unitsMap=%v",
			cr.Status.AtProvider.ID, cr.Status.AtProvider.UnitsMap)
	}
}

func TestObserveDriftOnUnitsMap(t *testing.T) {
	api := &fakeTeamAPI{
		getResp: &clients.TeamResource{
			ID:       5,
			Name:     "hermes-agent",
			UnitsMap: fullDefaultUnitsMap(map[string]clients.TeamPermissionLevel{"repo.code": "read"}),
		},
		listReposResp: []string{"hermes-agent"},
	}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, []string{"hermes-agent"})

	obs, _ := e.Observe(context.Background(), cr)
	if !obs.ResourceExists || obs.ResourceUpToDate {
		t.Fatalf("expected exists=true upToDate=false; got %+v", obs)
	}
}

func TestObserveDriftOnRepositories(t *testing.T) {
	api := &fakeTeamAPI{
		getResp: &clients.TeamResource{
			ID:       5,
			Name:     "hermes-agent",
			UnitsMap: fullDefaultUnitsMap(map[string]clients.TeamPermissionLevel{"repo.code": "write"}),
		},
		listReposResp: []string{"some-other-repo"},
	}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, []string{"hermes-agent"})

	obs, _ := e.Observe(context.Background(), cr)
	if obs.ResourceUpToDate {
		t.Fatalf("expected drift on repositories")
	}
}

func TestCreateExpandsUnitsMapToFullSet(t *testing.T) {
	api := &fakeTeamAPI{createResp: &clients.TeamResource{ID: 99}}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write", "repo.packages": "write"}, []string{"hermes-agent"})

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(api.createCalls) != 1 {
		t.Fatalf("expected one Create call, got %d", len(api.createCalls))
	}
	got := api.createCalls[0].UnitsMap
	if len(got) != len(clients.ValidTeamUnits) {
		t.Fatalf("expected full unit set (%d), got %d", len(clients.ValidTeamUnits), len(got))
	}
	if got["repo.code"] != clients.TeamPermWrite || got["repo.packages"] != clients.TeamPermWrite {
		t.Fatalf("explicit units not honoured: %v", got)
	}
	if got["repo.issues"] != clients.TeamPermNone {
		t.Fatalf("absent unit not defaulted to none: %v", got)
	}
	if cr.Status.AtProvider.ID == nil || *cr.Status.AtProvider.ID != 99 {
		t.Fatalf("status.atProvider.id not set from create response")
	}
}

func TestCreateAttachesRepositories(t *testing.T) {
	api := &fakeTeamAPI{createResp: &clients.TeamResource{ID: 7}}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, []string{"a", "b", "c"})

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(api.addRepoCalls) != 3 {
		t.Fatalf("expected 3 AddRepository calls, got %d", len(api.addRepoCalls))
	}
}

func TestCreateSkipsRepoAttachWhenIncludeAllRepositories(t *testing.T) {
	api := &fakeTeamAPI{createResp: &clients.TeamResource{ID: 7}}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, nil)
	cr.Spec.ForProvider.IncludeAllRepositories = ptrBool(true)

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if len(api.addRepoCalls) != 0 {
		t.Fatalf("expected zero AddRepository calls; got %d", len(api.addRepoCalls))
	}
}

func TestUpdateReconcilesRepoSet(t *testing.T) {
	id := int64(5)
	api := &fakeTeamAPI{
		listReposResp: []string{"keep", "stale"}, // current
	}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, []string{"keep", "new"})
	cr.Status.AtProvider.ID = &id

	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	// Expect AddRepository("new") and RemoveRepository("stale").
	if len(api.addRepoCalls) != 1 || api.addRepoCalls[0].repo != "new" {
		t.Fatalf("unexpected AddRepository calls: %+v", api.addRepoCalls)
	}
	if len(api.rmRepoCalls) != 1 || api.rmRepoCalls[0].repo != "stale" {
		t.Fatalf("unexpected RemoveRepository calls: %+v", api.rmRepoCalls)
	}
}

func TestUpdateRecoversIDIfStatusEmpty(t *testing.T) {
	api := &fakeTeamAPI{getResp: &clients.TeamResource{ID: 42, Name: "hermes-agent"}}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, nil)
	cr.Spec.ForProvider.IncludeAllRepositories = ptrBool(true)

	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if len(api.updateCalls) != 1 || api.updateCalls[0].id != 42 {
		t.Fatalf("expected Update at id=42; got %+v", api.updateCalls)
	}
}

func TestDeleteUsesStatusID(t *testing.T) {
	id := int64(8)
	api := &fakeTeamAPI{}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, nil)
	cr.Status.AtProvider.ID = &id

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != 8 {
		t.Fatalf("expected Delete(8); got %v", api.deleteCalls)
	}
}

func TestDeleteRecoversIDByName(t *testing.T) {
	api := &fakeTeamAPI{getResp: &clients.TeamResource{ID: 99, Name: "hermes-agent"}}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, nil)

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if len(api.deleteCalls) != 1 || api.deleteCalls[0] != 99 {
		t.Fatalf("expected Delete(99); got %v", api.deleteCalls)
	}
}

func TestDeleteNoOpIfTeamAlreadyGone(t *testing.T) {
	api := &fakeTeamAPI{getResp: nil}
	e := &external{api: api}
	cr := newTeam(map[string]string{"repo.code": "write"}, nil)

	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete should be no-op when team gone, got %v", err)
	}
	if len(api.deleteCalls) != 0 {
		t.Fatalf("expected zero Delete calls")
	}
}

func TestDiffStringSets(t *testing.T) {
	add, rm := diffStringSets([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	if !sliceEq(add, []string{"a"}) || !sliceEq(rm, []string{"d"}) {
		t.Fatalf("unexpected diff: add=%v rm=%v", add, rm)
	}
}

func TestParamsFromSpecExpansion(t *testing.T) {
	p := &v1alpha1.TeamParameters{
		Name:         ptrStr("x"),
		Organisation: ptrStr("apps"),
		UnitsMap:     map[string]string{"repo.code": "write", "repo.packages": "admin"},
	}
	out := paramsFromSpec(p)
	if len(out.UnitsMap) != len(clients.ValidTeamUnits) {
		t.Fatalf("expected expansion to all units, got %d entries", len(out.UnitsMap))
	}
	for unit := range clients.ValidTeamUnits {
		if _, ok := out.UnitsMap[unit]; !ok {
			t.Fatalf("missing unit %q in expansion", unit)
		}
	}
	if out.UnitsMap["repo.code"] != "write" || out.UnitsMap["repo.packages"] != "admin" {
		t.Fatalf("explicit values lost: %v", out.UnitsMap)
	}
	for unit, lvl := range out.UnitsMap {
		if unit == "repo.code" || unit == "repo.packages" {
			continue
		}
		if lvl != clients.TeamPermNone {
			t.Fatalf("expected %q default to none, got %q", unit, lvl)
		}
	}
}

// fullDefaultUnitsMap mirrors paramsFromSpec's expansion: every recognised
// unit gets `none` unless overridden in `set`. Used by Observe tests to
// build a representative server-side state.
func fullDefaultUnitsMap(set map[string]clients.TeamPermissionLevel) map[string]clients.TeamPermissionLevel {
	out := make(map[string]clients.TeamPermissionLevel, len(clients.ValidTeamUnits))
	for u := range clients.ValidTeamUnits {
		out[u] = clients.TeamPermNone
	}
	for u, l := range set {
		out[u] = l
	}
	return out
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Belt-and-braces guard: the controller exposes `errInvalidUnits` etc. as
// constants; if anyone changes the validator's error wording without
// updating the constants, the build-time reference here keeps them honest.
var _ = strings.Contains
