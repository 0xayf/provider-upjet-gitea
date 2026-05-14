// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

// TokenAPI is the minimal HTTP surface the Token controller needs.
//
// Gitea's /users/{username}/tokens endpoints accept only HTTP Basic auth as
// the user the token belongs to — bearer auth and admin-sudo are both
// refused (see https://gitea.com/gitea/go-sdk/issues/610). The
// ProviderConfig referenced by the Token MR therefore must point at a
// credentials secret containing that user's username + password.
type TokenAPI interface {
	// List returns all tokens for the authenticated user. The token value
	// is unrecoverable from this endpoint — it is only returned at create
	// time.
	List(ctx context.Context, username string) ([]TokenResource, error)
	// Create mints a new token. Returns the raw token value (sha1) in
	// the response — this is the only opportunity to capture it.
	Create(ctx context.Context, username, name string, scopes []string) (*TokenCreateResponse, error)
	// Delete removes a token by its numeric ID. 404 is treated as
	// success (idempotent delete).
	Delete(ctx context.Context, username string, id int64) error
}

// TokenResource is the shape returned by GET /users/{username}/tokens.
// The `sha1` value is empty in list responses — it is only populated on
// create.
type TokenResource struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// TokenCreateResponse is the shape returned by POST /users/{username}/tokens.
type TokenCreateResponse struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	// Sha1 is the raw token value. Persist it immediately — Gitea never
	// returns it again.
	Sha1 string `json:"sha1"`
}

// NewGiteaTokenClient returns an HTTP-backed implementation. The credentials
// passed in must have Username and Password set; Token-only credentials
// cannot drive this endpoint family.
func NewGiteaTokenClient(creds *GiteaCreds) TokenAPI {
	return &giteaTokenClient{
		creds: creds,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

type giteaTokenClient struct {
	creds *GiteaCreds
	http  *http.Client
}

func (c *giteaTokenClient) authenticate(req *http.Request) {
	if c.creds.Username == "" {
		return
	}
	// Basic auth is required for /users/{username}/tokens. If the
	// credentials secret carries only a token, fall back to using that as
	// the password — works when the token has write:user scope.
	password := c.creds.Password
	if password == "" {
		password = c.creds.Token
	}
	req.SetBasicAuth(c.creds.Username, password)
}

func (c *giteaTokenClient) List(ctx context.Context, username string) ([]TokenResource, error) {
	path := fmt.Sprintf("/api/v1/users/%s/tokens", url.PathEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.creds.Endpoint+path, nil)
	if err != nil {
		return nil, err
	}
	c.authenticate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list tokens returned %d: %s", resp.StatusCode, string(body))
	}
	var items []TokenResource
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, errors.Wrap(err, "decode list response")
	}
	return items, nil
}

func (c *giteaTokenClient) Create(ctx context.Context, username, name string, scopes []string) (*TokenCreateResponse, error) {
	path := fmt.Sprintf("/api/v1/users/%s/tokens", url.PathEscape(username))
	body, err := json.Marshal(map[string]any{"name": name, "scopes": scopes})
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.creds.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authenticate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create token returned %d: %s", resp.StatusCode, string(respBody))
	}
	var out TokenCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "decode create response")
	}
	if out.Sha1 == "" {
		return nil, errors.New("create response missing sha1 (token value) — Gitea API change?")
	}
	return &out, nil
}

func (c *giteaTokenClient) Delete(ctx context.Context, username string, id int64) error {
	path := fmt.Sprintf("/api/v1/users/%s/tokens/%d", url.PathEscape(username), id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.creds.Endpoint+path, nil)
	if err != nil {
		return err
	}
	c.authenticate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("delete token returned %d: %s", resp.StatusCode, string(respBody))
}
