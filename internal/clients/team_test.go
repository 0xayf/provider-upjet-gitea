// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func newTeamClient(endpoint, token string) TeamAPI {
	return NewGiteaTeamClient(&GiteaCreds{Endpoint: endpoint, Token: token})
}

func TestTeamGetByNameFound(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": []map[string]any{
				{"id": 5, "name": "hermes-agent", "permission": "none",
					"units":     []string{"repo.code", "repo.packages"},
					"units_map": map[string]string{"repo.code": "write", "repo.packages": "write"}},
			},
		})
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.Get(context.Background(), "apps", "hermes-agent")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got == nil || got.ID != 5 || got.Name != "hermes-agent" {
		t.Fatalf("unexpected team: %#v", got)
	}
	if got.UnitsMap["repo.packages"] != TeamPermWrite {
		t.Fatalf("expected repo.packages=write, got %q", got.UnitsMap["repo.packages"])
	}
	if (*recs)[0].Path != "/api/v1/orgs/apps/teams/search" {
		t.Fatalf("unexpected path: %s", (*recs)[0].Path)
	}
	if (*recs)[0].Auth != "token tok" {
		t.Fatalf("auth missing or wrong: %q", (*recs)[0].Auth)
	}
}

func TestTeamGetByNameMissing(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"data": []map[string]any{},
		})
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.Get(context.Background(), "apps", "no-such-team")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestTeamGetByNameMatchesExactNameNotPrefix(t *testing.T) {
	// Search returns prefix matches; Get should only return exact-name match.
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": []map[string]any{
				{"id": 1, "name": "hermes-agent-other"},
				{"id": 2, "name": "hermes-agent-yet-another"},
			},
		})
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.Get(context.Background(), "apps", "hermes-agent")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for non-exact match, got %#v", got)
	}
}

func TestTeamGetByID(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         5,
			"name":       "hermes-agent",
			"permission": "none",
			"units_map":  map[string]string{"repo.packages": "write"},
		})
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.GetByID(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got == nil || got.ID != 5 {
		t.Fatalf("unexpected: %#v", got)
	}
	if (*recs)[0].Path != "/api/v1/teams/5" {
		t.Fatalf("unexpected path: %s", (*recs)[0].Path)
	}
}

func TestTeamGetByIDMissing(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.GetByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for 404, got %#v", got)
	}
}

func TestTeamCreateSuccess(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 42, "name": "hermes-agent",
			"units_map": map[string]string{"repo.code": "write", "repo.packages": "write"},
		})
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.Create(context.Background(), "apps", TeamParams{
		Name: "hermes-agent",
		UnitsMap: map[string]TeamPermissionLevel{
			"repo.code":     TeamPermWrite,
			"repo.packages": TeamPermWrite,
		},
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if got.ID != 42 {
		t.Fatalf("unexpected id: %d", got.ID)
	}
	if (*recs)[0].Method != http.MethodPost || (*recs)[0].Path != "/api/v1/orgs/apps/teams" {
		t.Fatalf("unexpected request: %#v", (*recs)[0])
	}
	if !strings.Contains((*recs)[0].Body, `"units_map"`) {
		t.Fatalf("body missing units_map: %s", (*recs)[0].Body)
	}
}

func TestTeamCreateRejectsEmptyUnitsMap(t *testing.T) {
	client := newTeamClient("http://example.invalid", "tok")
	_, err := client.Create(context.Background(), "apps", TeamParams{
		Name:     "x",
		UnitsMap: map[string]TeamPermissionLevel{},
	})
	if err == nil || !strings.Contains(err.Error(), "unitsMap is required") {
		t.Fatalf("expected unitsMap required error, got %v", err)
	}
}

func TestTeamCreateRejectsEmptyName(t *testing.T) {
	client := newTeamClient("http://example.invalid", "tok")
	_, err := client.Create(context.Background(), "apps", TeamParams{
		UnitsMap: map[string]TeamPermissionLevel{"repo.code": TeamPermRead},
	})
	if err == nil || !strings.Contains(err.Error(), "team name is required") {
		t.Fatalf("expected name error, got %v", err)
	}
}

func TestTeamCreateRejectsUnknownUnit(t *testing.T) {
	client := newTeamClient("http://example.invalid", "tok")
	_, err := client.Create(context.Background(), "apps", TeamParams{
		Name:     "x",
		UnitsMap: map[string]TeamPermissionLevel{"repo.bogus": TeamPermWrite},
	})
	if err == nil || !strings.Contains(err.Error(), "not a recognised") {
		t.Fatalf("expected unknown unit error, got %v", err)
	}
}

func TestTeamCreateRejectsInvalidLevel(t *testing.T) {
	client := newTeamClient("http://example.invalid", "tok")
	_, err := client.Create(context.Background(), "apps", TeamParams{
		Name:     "x",
		UnitsMap: map[string]TeamPermissionLevel{"repo.code": "owner"},
	})
	if err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("expected level error, got %v", err)
	}
}

func TestTeamCreateAcceptsAllValidLevels(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "x"})
	})
	client := newTeamClient(srv.URL, "tok")
	for _, lvl := range []TeamPermissionLevel{TeamPermNone, TeamPermRead, TeamPermWrite, TeamPermAdmin} {
		_, err := client.Create(context.Background(), "apps", TeamParams{
			Name:     "x-" + string(lvl),
			UnitsMap: map[string]TeamPermissionLevel{"repo.code": lvl},
		})
		if err != nil {
			t.Fatalf("level %s rejected: %v", lvl, err)
		}
	}
}

func TestTeamCreateServerError(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	})
	client := newTeamClient(srv.URL, "tok")
	_, err := client.Create(context.Background(), "apps", TeamParams{
		Name:     "x",
		UnitsMap: map[string]TeamPermissionLevel{"repo.code": TeamPermWrite},
	})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}

func TestTeamUpdateSendsPATCH(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 5, "name": "hermes-agent"})
	})
	client := newTeamClient(srv.URL, "tok")
	_, err := client.Update(context.Background(), 5, TeamParams{
		Name:     "hermes-agent",
		UnitsMap: map[string]TeamPermissionLevel{"repo.packages": TeamPermWrite},
	})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if (*recs)[0].Method != http.MethodPatch || (*recs)[0].Path != "/api/v1/teams/5" {
		t.Fatalf("unexpected request: %#v", (*recs)[0])
	}
}

func TestTeamDeleteSuccess(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := newTeamClient(srv.URL, "tok")
	if err := client.Delete(context.Background(), 5); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
}

func TestTeamDeleteIdempotentOn404(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := newTeamClient(srv.URL, "tok")
	if err := client.Delete(context.Background(), 999); err != nil {
		t.Fatalf("Delete should be idempotent on 404, got: %v", err)
	}
}

func TestTeamAddRepository(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := newTeamClient(srv.URL, "tok")
	if err := client.AddRepository(context.Background(), 5, "hermes-agent"); err != nil {
		t.Fatalf("AddRepository error: %v", err)
	}
	if (*recs)[0].Method != http.MethodPut || (*recs)[0].Path != "/api/v1/teams/5/repos/hermes-agent" {
		t.Fatalf("unexpected request: %#v", (*recs)[0])
	}
}

func TestTeamRemoveRepositoryIdempotentOn404(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := newTeamClient(srv.URL, "tok")
	if err := client.RemoveRepository(context.Background(), 5, "ghost-repo"); err != nil {
		t.Fatalf("RemoveRepository should be idempotent on 404, got: %v", err)
	}
}

func TestTeamListRepositoriesPaginates(t *testing.T) {
	page := 0
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		page++
		switch page {
		case 1:
			items := make([]map[string]string, 50)
			for i := range items {
				items[i] = map[string]string{"name": fmt.Sprintf("repo-%02d", i)}
			}
			_ = json.NewEncoder(w).Encode(items)
		case 2:
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"name": "repo-50"},
				{"name": "repo-51"},
			})
		}
	})
	client := newTeamClient(srv.URL, "tok")
	got, err := client.ListRepositories(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListRepositories error: %v", err)
	}
	if len(got) != 52 {
		t.Fatalf("expected 52 repos across two pages, got %d", len(got))
	}
}

func TestTeamIsMemberTrue(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "alice"})
	})
	client := newTeamClient(srv.URL, "tok")
	ok, err := client.IsMember(context.Background(), 5, "alice")
	if err != nil {
		t.Fatalf("IsMember error: %v", err)
	}
	if !ok {
		t.Fatalf("expected true")
	}
	if (*recs)[0].Path != "/api/v1/teams/5/members/alice" {
		t.Fatalf("unexpected path: %s", (*recs)[0].Path)
	}
}

func TestTeamIsMemberFalseOn404(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := newTeamClient(srv.URL, "tok")
	ok, err := client.IsMember(context.Background(), 5, "ghost")
	if err != nil {
		t.Fatalf("IsMember error: %v", err)
	}
	if ok {
		t.Fatalf("expected false")
	}
}

func TestTeamAddMember(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := newTeamClient(srv.URL, "tok")
	if err := client.AddMember(context.Background(), 5, "alice"); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}
	if (*recs)[0].Method != http.MethodPut || (*recs)[0].Path != "/api/v1/teams/5/members/alice" {
		t.Fatalf("unexpected request: %#v", (*recs)[0])
	}
}

func TestTeamRemoveMemberIdempotentOn404(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := newTeamClient(srv.URL, "tok")
	if err := client.RemoveMember(context.Background(), 5, "ghost"); err != nil {
		t.Fatalf("RemoveMember should be idempotent on 404, got: %v", err)
	}
}

func TestTeamUsesBasicAuthIfNoToken(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": []map[string]any{}})
	})
	client := NewGiteaTeamClient(&GiteaCreds{Endpoint: srv.URL, Username: "admin", Password: "p"})
	_, _ = client.Get(context.Background(), "apps", "anything")
	if !strings.HasPrefix((*recs)[0].Auth, "Basic ") {
		t.Fatalf("expected Basic auth, got %q", (*recs)[0].Auth)
	}
}
