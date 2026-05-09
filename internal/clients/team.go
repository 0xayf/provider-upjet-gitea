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
	"sort"
	"time"

	"github.com/pkg/errors"
)

// TeamPermissionLevel is the per-unit access level a team is granted on a
// repository. Mirrors Gitea's role names in the team API.
type TeamPermissionLevel string

const (
	TeamPermNone  TeamPermissionLevel = "none"
	TeamPermRead  TeamPermissionLevel = "read"
	TeamPermWrite TeamPermissionLevel = "write"
	TeamPermAdmin TeamPermissionLevel = "admin"
)

// IsValid reports whether the level is one of the values Gitea accepts on
// custom teams.
func (l TeamPermissionLevel) IsValid() bool {
	switch l {
	case TeamPermNone, TeamPermRead, TeamPermWrite, TeamPermAdmin:
		return true
	}
	return false
}

// ValidTeamUnits is the set of unit names Gitea recognises on a team. Keys
// outside this set produce a validation error before reaching the API.
var ValidTeamUnits = map[string]struct{}{
	"repo.code":       {},
	"repo.issues":     {},
	"repo.ext_issues": {},
	"repo.wiki":       {},
	"repo.ext_wiki":   {},
	"repo.pulls":      {},
	"repo.releases":   {},
	"repo.projects":   {},
	"repo.packages":   {},
	"repo.actions":    {},
}

// TeamAPI is the surface a controller needs for managing Gitea teams with
// per-unit permissions. An interface keeps the controller fake-able in tests.
type TeamAPI interface {
	// Get returns the team named name in org, or (nil, nil) if absent. Errors
	// are HTTP/transport-level (auth, network, server).
	Get(ctx context.Context, org, name string) (*TeamResource, error)
	// GetByID fetches a team by its numeric Gitea ID. Used after Create to
	// fetch the canonical representation. Returns (nil, nil) if absent.
	GetByID(ctx context.Context, id int64) (*TeamResource, error)
	// Create creates a new team and returns its observed state.
	Create(ctx context.Context, org string, params TeamParams) (*TeamResource, error)
	// Update replaces the team at id with the given params. Gitea's PATCH
	// endpoint takes the full set of fields each time.
	Update(ctx context.Context, id int64, params TeamParams) (*TeamResource, error)
	// Delete removes the team at id. Returns nil if it doesn't exist.
	Delete(ctx context.Context, id int64) error

	// AddRepository attaches a repo to a team's repositories list.
	AddRepository(ctx context.Context, id int64, repo string) error
	// RemoveRepository detaches a repo from a team's repositories list.
	RemoveRepository(ctx context.Context, id int64, repo string) error
	// ListRepositories returns the repos currently attached to a team.
	ListRepositories(ctx context.Context, id int64) ([]string, error)

	// IsMember reports whether username is a current member of team id. Returns
	// (false, nil) for both "not in team" and "user does not exist".
	IsMember(ctx context.Context, id int64, username string) (bool, error)
	// AddMember adds username to team id. No-op if already a member.
	AddMember(ctx context.Context, id int64, username string) error
	// RemoveMember removes username from team id. Idempotent.
	RemoveMember(ctx context.Context, id int64, username string) error
}

// TeamParams are the writable fields on a Gitea team. UnitsMap drives per-unit
// permissions; Gitea derives the legacy `permission` field from it on read.
type TeamParams struct {
	Name                   string                         `json:"name"`
	Description            string                         `json:"description,omitempty"`
	IncludesAllRepositories bool                          `json:"includes_all_repositories"`
	CanCreateOrgRepo       bool                           `json:"can_create_org_repo"`
	UnitsMap               map[string]TeamPermissionLevel `json:"units_map"`
}

// TeamResource is the read-side shape returned by Gitea.
type TeamResource struct {
	ID                      int64                          `json:"id"`
	Name                    string                         `json:"name"`
	Description             string                         `json:"description"`
	IncludesAllRepositories bool                           `json:"includes_all_repositories"`
	CanCreateOrgRepo        bool                           `json:"can_create_org_repo"`
	Permission              string                         `json:"permission"`
	Units                   []string                       `json:"units"`
	UnitsMap                map[string]TeamPermissionLevel `json:"units_map"`
	Organization            *struct {
		Username string `json:"username"`
	} `json:"organization,omitempty"`
}

// NewGiteaTeamClient returns an HTTP-backed TeamAPI implementation.
func NewGiteaTeamClient(creds *GiteaCreds) TeamAPI {
	return &giteaTeamClient{
		creds: creds,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

type giteaTeamClient struct {
	creds *GiteaCreds
	http  *http.Client
}

func (c *giteaTeamClient) authenticate(req *http.Request) {
	if c.creds.Token != "" {
		req.Header.Set("Authorization", "token "+c.creds.Token)
		return
	}
	if c.creds.Username != "" {
		req.SetBasicAuth(c.creds.Username, c.creds.Password)
	}
}

func (c *giteaTeamClient) do(req *http.Request) (*http.Response, error) {
	c.authenticate(req)
	return c.http.Do(req)
}

// orgTeamSearchPath returns the path that lists teams in an org filtered by
// name. We use search rather than the bulk list to keep the payload small.
func orgTeamSearchPath(org, name string) string {
	q := url.Values{}
	q.Set("q", name)
	return fmt.Sprintf("/api/v1/orgs/%s/teams/search?%s", url.PathEscape(org), q.Encode())
}

func (c *giteaTeamClient) Get(ctx context.Context, org, name string) (*TeamResource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.creds.Endpoint+orgTeamSearchPath(org, name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search teams returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		OK   bool           `json:"ok"`
		Data []TeamResource `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "decode search response")
	}
	for i := range result.Data {
		if result.Data[i].Name == name {
			return &result.Data[i], nil
		}
	}
	return nil, nil
}

func (c *giteaTeamClient) GetByID(ctx context.Context, id int64) (*TeamResource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/teams/%d", c.creds.Endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get team returned %d: %s", resp.StatusCode, string(body))
	}
	var t TeamResource
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, errors.Wrap(err, "decode team response")
	}
	return &t, nil
}

func (c *giteaTeamClient) Create(ctx context.Context, org string, params TeamParams) (*TeamResource, error) {
	if err := validateTeamParams(params); err != nil {
		return nil, err
	}
	body, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Wrap(err, "marshal create body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/orgs/%s/teams", c.creds.Endpoint, url.PathEscape(org)),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create team returned %d: %s", resp.StatusCode, string(respBody))
	}
	var t TeamResource
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, errors.Wrap(err, "decode create response")
	}
	return &t, nil
}

func (c *giteaTeamClient) Update(ctx context.Context, id int64, params TeamParams) (*TeamResource, error) {
	if err := validateTeamParams(params); err != nil {
		return nil, err
	}
	body, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Wrap(err, "marshal update body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		fmt.Sprintf("%s/api/v1/teams/%d", c.creds.Endpoint, id),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update team returned %d: %s", resp.StatusCode, string(respBody))
	}
	var t TeamResource
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, errors.Wrap(err, "decode update response")
	}
	return &t, nil
}

func (c *giteaTeamClient) Delete(ctx context.Context, id int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/teams/%d", c.creds.Endpoint, id), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("delete team returned %d: %s", resp.StatusCode, string(respBody))
}

func (c *giteaTeamClient) AddRepository(ctx context.Context, id int64, repo string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/v1/teams/%d/repos/%s", c.creds.Endpoint, id, url.PathEscape(repo)), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("add repo to team returned %d: %s", resp.StatusCode, string(respBody))
}

func (c *giteaTeamClient) RemoveRepository(ctx context.Context, id int64, repo string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/teams/%d/repos/%s", c.creds.Endpoint, id, url.PathEscape(repo)), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("remove repo from team returned %d: %s", resp.StatusCode, string(respBody))
}

func (c *giteaTeamClient) ListRepositories(ctx context.Context, id int64) ([]string, error) {
	// Page through results — Gitea defaults to 50 per page, max 50.
	out := []string{}
	page := 1
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/api/v1/teams/%d/repos?page=%d&limit=50", c.creds.Endpoint, id, page), nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil, nil
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("list team repos returned %d: %s", resp.StatusCode, string(body))
		}

		var items []struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			resp.Body.Close()
			return nil, errors.Wrap(err, "decode list repos response")
		}
		resp.Body.Close()

		for _, it := range items {
			out = append(out, it.Name)
		}
		if len(items) < 50 {
			break
		}
		page++
	}
	sort.Strings(out)
	return out, nil
}

func (c *giteaTeamClient) IsMember(ctx context.Context, id int64, username string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/teams/%d/members/%s", c.creds.Endpoint, id, url.PathEscape(username)), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("get team member returned %d: %s", resp.StatusCode, string(respBody))
}

func (c *giteaTeamClient) AddMember(ctx context.Context, id int64, username string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/v1/teams/%d/members/%s", c.creds.Endpoint, id, url.PathEscape(username)), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("add team member returned %d: %s", resp.StatusCode, string(respBody))
}

func (c *giteaTeamClient) RemoveMember(ctx context.Context, id int64, username string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/teams/%d/members/%s", c.creds.Endpoint, id, url.PathEscape(username)), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("remove team member returned %d: %s", resp.StatusCode, string(respBody))
}

// validateTeamParams enforces the small client-side rules Gitea would reject
// at the API layer too — caught earlier here for cleaner error messages.
func validateTeamParams(p TeamParams) error {
	if p.Name == "" {
		return errors.New("team name is required")
	}
	if len(p.UnitsMap) == 0 {
		return errors.New("unitsMap is required and must contain at least one unit")
	}
	for unit, level := range p.UnitsMap {
		if _, ok := ValidTeamUnits[unit]; !ok {
			return fmt.Errorf("unitsMap key %q is not a recognised Gitea team unit", unit)
		}
		if !level.IsValid() {
			return fmt.Errorf("unitsMap[%q] level %q is not valid (none|read|write|admin)", unit, level)
		}
	}
	return nil
}
