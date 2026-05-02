// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clients

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// roundTrip records how a request looked, so tests can assert path/auth/body.
type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   string
}

// fakeServer returns an *httptest.Server, a *[]recordedRequest of every
// inbound request, and a handler the test customises per case.
func fakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, rec *recordedRequest)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var recs []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec := recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Body:   string(body),
		}
		recs = append(recs, rec)
		handler(w, r, &rec)
	}))
	t.Cleanup(srv.Close)
	return srv, &recs
}

func newClient(endpoint, token string) ActionsSecretAPI {
	return NewGiteaActionsSecretClient(&GiteaCreds{Endpoint: endpoint, Token: token})
}

func ptrStr(s string) *string { return &s }

func TestPutOrgSecretSuccess(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("apps")}
	if err := client.Put(context.Background(), scope, "CI_BOT_TOKEN", "secret-value"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if len(*recs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*recs))
	}
	r := (*recs)[0]
	if r.Method != http.MethodPut {
		t.Fatalf("expected PUT, got %s", r.Method)
	}
	if r.Path != "/api/v1/orgs/apps/actions/secrets/CI_BOT_TOKEN" {
		t.Fatalf("unexpected path: %s", r.Path)
	}
	if r.Auth != "token tok" {
		t.Fatalf("unexpected auth header: %s", r.Auth)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if body["data"] != "secret-value" {
		t.Fatalf("expected body.data=secret-value, got %v", body)
	}
}

func TestPutRepoSecretSuccess(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusCreated)
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Repository: &RepoTarget{Owner: "apps", Name: "hermes-agent"}}
	if err := client.Put(context.Background(), scope, "DEPLOY_KEY", "v"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if (*recs)[0].Path != "/api/v1/repos/apps/hermes-agent/actions/secrets/DEPLOY_KEY" {
		t.Fatalf("unexpected path: %s", (*recs)[0].Path)
	}
}

func TestPutUserSecretSuccess(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{User: true}
	if err := client.Put(context.Background(), scope, "MY_TOKEN", "v"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if (*recs)[0].Path != "/api/v1/user/actions/secrets/MY_TOKEN" {
		t.Fatalf("unexpected path: %s", (*recs)[0].Path)
	}
}

func TestPutErrorPropagatesGiteaMessage(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"permission denied"}`))
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("apps")}
	err := client.Put(context.Background(), scope, "x", "v")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected gitea message in error, got: %v", err)
	}
}

func TestGetReturnsNilWhenNotInList(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_, _ = w.Write([]byte(`[{"name":"OTHER","created_at":"2026-01-01T00:00:00Z"}]`))
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("apps")}
	got, err := client.Get(context.Background(), scope, "MISSING")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing secret, got %+v", got)
	}
}

func TestGetReturnsMatchFromList(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		_, _ = w.Write([]byte(`[{"name":"CI_BOT_TOKEN","created_at":"2026-01-01T00:00:00Z"},{"name":"OTHER"}]`))
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("apps")}
	got, err := client.Get(context.Background(), scope, "CI_BOT_TOKEN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "CI_BOT_TOKEN" {
		t.Fatalf("expected CI_BOT_TOKEN, got %+v", got)
	}
	if got.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected createdAt: %s", got.CreatedAt)
	}
}

func TestGetReturnsNilOn404(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("nonexistent-org")}
	got, err := client.Get(context.Background(), scope, "X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for 404 scope, got %+v", got)
	}
}

func TestDeleteSucceedsOn204(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{User: true}
	if err := client.Delete(context.Background(), scope, "X"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if (*recs)[0].Method != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", (*recs)[0].Method)
	}
}

func TestDeleteSucceedsOn404(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNotFound)
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("apps")}
	if err := client.Delete(context.Background(), scope, "X"); err != nil {
		t.Fatalf("expected nil error for 404, got: %v", err)
	}
}

func TestUsesBasicAuthWhenTokenAbsent(t *testing.T) {
	srv, recs := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := NewGiteaActionsSecretClient(&GiteaCreds{
		Endpoint: srv.URL,
		Username: "admin",
		Password: "p",
	})
	scope := ActionsSecretScope{Organization: ptrStr("apps")}
	if err := client.Put(context.Background(), scope, "X", "v"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	auth := (*recs)[0].Auth
	if !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("expected Basic auth, got: %s", auth)
	}
}

func TestPathEscapesSpecialCharacters(t *testing.T) {
	// We need to inspect the raw RequestURI rather than r.URL.Path,
	// since net/http decodes escapes when populating Path.
	var rawURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawURI = r.RequestURI
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{Organization: ptrStr("my org")}
	if err := client.Put(context.Background(), scope, "name with space", "v"); err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if !strings.Contains(rawURI, "my%20org") {
		t.Fatalf("expected escaped org in path, got: %s", rawURI)
	}
	if !strings.Contains(rawURI, "name%20with%20space") {
		t.Fatalf("expected escaped name in path, got: %s", rawURI)
	}
}

func TestRejectsInvalidScope(t *testing.T) {
	srv, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		t.Fatalf("server should not be called for invalid scope")
	})

	client := newClient(srv.URL, "tok")
	scope := ActionsSecretScope{} // no fields set
	if err := client.Put(context.Background(), scope, "X", "v"); err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if err := client.Delete(context.Background(), scope, "X"); err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if _, err := client.Get(context.Background(), scope, "X"); err == nil {
		t.Fatal("expected error for invalid scope")
	}
}
