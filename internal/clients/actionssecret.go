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

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GiteaCreds is a flattened view of the credentials a managed-resource
// controller needs to call the Gitea API. It is derived from a ProviderConfig
// + its referenced credentials secret.
type GiteaCreds struct {
	// Endpoint is the Gitea base URL (no trailing slash).
	Endpoint string

	// Username/Password authenticate via HTTP Basic. Mutually exclusive with
	// Token in practice; if both are set, Token wins for endpoints that
	// accept it.
	Username string
	Password string

	// Token is a Gitea personal access token. Used as the password when basic
	// auth is required (e.g. /users/{username}/tokens) or as
	// `Authorization: token <Token>` for API endpoints that accept it.
	Token string
}

// ResolveGiteaCreds resolves the ProviderConfig referenced by mg and returns
// a flattened set of credentials suitable for direct HTTP calls to Gitea.
//
// Endpoint precedence: ProviderConfig.spec.endpoint wins; otherwise base_url
// from the credentials secret is used (back-compat).
func ResolveGiteaCreds(ctx context.Context, kube client.Client, mg resource.Managed) (*GiteaCreds, error) {
	pcSpec, err := resolveProviderConfig(ctx, kube, mg)
	if err != nil {
		return nil, errors.Wrap(err, "cannot resolve provider config")
	}

	data, err := resource.CommonCredentialExtractor(ctx, pcSpec.Credentials.Source, kube, pcSpec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errExtractCredentials)
	}

	creds := map[string]string{}
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, errors.Wrap(err, errUnmarshalCredentials)
	}

	g := &GiteaCreds{
		Username: creds["username"],
		Password: creds["password"],
		Token:    creds["token"],
	}
	if pcSpec.Endpoint != "" {
		g.Endpoint = pcSpec.Endpoint
	} else {
		g.Endpoint = creds["base_url"]
	}
	if g.Endpoint == "" {
		return nil, errors.New("provider config has no endpoint and credentials secret has no base_url")
	}
	return g, nil
}

// ActionsSecretAPI is the minimal HTTP surface the controllers need for
// managing Gitea Actions secrets at any scope. Keeping it as an interface
// makes the controllers testable with a fake.
type ActionsSecretAPI interface {
	Get(ctx context.Context, scope ActionsSecretScope, name string) (*ActionsSecretResource, error)
	Put(ctx context.Context, scope ActionsSecretScope, name, value string) error
	Delete(ctx context.Context, scope ActionsSecretScope, name string) error
}

// ActionsSecretScope identifies which Gitea endpoint family the operation
// targets. Only one of Repository, Organization, or User should be populated.
type ActionsSecretScope struct {
	// Repository is set for repo-scoped secrets.
	Repository *RepoTarget
	// Organization is set for org-scoped secrets.
	Organization *string
	// User is set for user-scoped secrets (path is /user/..., the
	// authenticated user is implied).
	User bool
}

// RepoTarget identifies a single repository.
type RepoTarget struct {
	Owner string
	Name  string
}

// ActionsSecretResource is what GET returns - or, for endpoints that don't
// expose a GET, what we synthesise to indicate existence.
type ActionsSecretResource struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
}

// NewGiteaActionsSecretClient returns an HTTP-backed implementation.
func NewGiteaActionsSecretClient(creds *GiteaCreds) ActionsSecretAPI {
	return &giteaActionsSecretClient{
		creds: creds,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

type giteaActionsSecretClient struct {
	creds *GiteaCreds
	http  *http.Client
}

// pathFor builds the API path for the given scope + secret name. It returns
// "" if the scope is invalid (no field set).
func (c *giteaActionsSecretClient) pathFor(scope ActionsSecretScope, name string) string {
	enc := url.PathEscape(name)
	switch {
	case scope.Repository != nil:
		return fmt.Sprintf("/api/v1/repos/%s/%s/actions/secrets/%s",
			url.PathEscape(scope.Repository.Owner),
			url.PathEscape(scope.Repository.Name),
			enc)
	case scope.Organization != nil:
		return fmt.Sprintf("/api/v1/orgs/%s/actions/secrets/%s",
			url.PathEscape(*scope.Organization), enc)
	case scope.User:
		return fmt.Sprintf("/api/v1/user/actions/secrets/%s", enc)
	}
	return ""
}

// listPathFor builds the API path that lists secrets at the given scope. Used
// to implement Get since Gitea's PUT/DELETE-by-name endpoints do not expose
// a corresponding GET - we list and filter.
func (c *giteaActionsSecretClient) listPathFor(scope ActionsSecretScope) string {
	switch {
	case scope.Repository != nil:
		return fmt.Sprintf("/api/v1/repos/%s/%s/actions/secrets",
			url.PathEscape(scope.Repository.Owner),
			url.PathEscape(scope.Repository.Name))
	case scope.Organization != nil:
		return fmt.Sprintf("/api/v1/orgs/%s/actions/secrets", url.PathEscape(*scope.Organization))
	case scope.User:
		return "/api/v1/user/actions/secrets"
	}
	return ""
}

func (c *giteaActionsSecretClient) authenticate(req *http.Request) {
	if c.creds.Token != "" {
		req.Header.Set("Authorization", "token "+c.creds.Token)
		return
	}
	if c.creds.Username != "" {
		req.SetBasicAuth(c.creds.Username, c.creds.Password)
	}
}

func (c *giteaActionsSecretClient) do(req *http.Request) (*http.Response, error) {
	c.authenticate(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Get returns the secret if it exists, or (nil, nil) if it does not. Other
// errors (auth, network, server) are returned as errors.
func (c *giteaActionsSecretClient) Get(ctx context.Context, scope ActionsSecretScope, name string) (*ActionsSecretResource, error) {
	listPath := c.listPathFor(scope)
	if listPath == "" {
		return nil, errors.New("invalid scope: no target field set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.creds.Endpoint+listPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Scope itself doesn't exist (org/repo missing).
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list actions secrets returned %d: %s", resp.StatusCode, string(body))
	}

	var items []ActionsSecretResource
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, errors.Wrap(err, "decode list response")
	}
	for _, s := range items {
		if s.Name == name {
			return &s, nil
		}
	}
	return nil, nil
}

// Put creates or updates a secret. Gitea uses the same PUT verb for both;
// 201 Created or 204 No Content are both success codes.
func (c *giteaActionsSecretClient) Put(ctx context.Context, scope ActionsSecretScope, name, value string) error {
	path := c.pathFor(scope, name)
	if path == "" {
		return errors.New("invalid scope: no target field set")
	}
	body, err := json.Marshal(map[string]string{"data": value})
	if err != nil {
		return errors.Wrap(err, "marshal request body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.creds.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("put actions secret returned %d: %s", resp.StatusCode, string(respBody))
}

// Delete removes a secret. Returns nil if the secret did not exist.
func (c *giteaActionsSecretClient) Delete(ctx context.Context, scope ActionsSecretScope, name string) error {
	path := c.pathFor(scope, name)
	if path == "" {
		return errors.New("invalid scope: no target field set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.creds.Endpoint+path, nil)
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
	return fmt.Errorf("delete actions secret returned %d: %s", resp.StatusCode, string(respBody))
}
