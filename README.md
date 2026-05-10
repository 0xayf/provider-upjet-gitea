# provider-upjet-gitea

A Crossplane provider for [Gitea](https://gitea.com). Most resources are
generated with [Upjet](https://github.com/crossplane/upjet) from the
[upstream Terraform provider](https://github.com/go-gitea/terraform-provider-gitea);
a small number — `Team`, `Membership`, `OrgActionsSecret`,
`UserActionsSecret` — are hand-written so the controller can talk to the
Gitea API directly where the Terraform shape is too lossy.

Both cluster-scoped and namespaced variants of every resource are shipped.
The namespaced variants live under the `*.gitea.m.crossplane.io` API
groups and are the right choice for compositions and Crossplane v2
namespaced flows.

## Install

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-upjet-gitea
spec:
  package: ghcr.io/0xayf/provider-upjet-gitea:v0.3.2
```

## ProviderConfig

```yaml
apiVersion: gitea.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: gitea
spec:
  endpoint: https://gitea.example.com
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: gitea-credentials
      key: credentials
```

The credentials secret is JSON. Use a token:

```json
{ "token": "your-access-token" }
```

…or username/password (required by the `Token` resource, since Gitea's
`/users/{username}/tokens` endpoint only accepts basic auth):

```json
{ "username": "admin", "password": "your-password" }
```

A legacy `base_url` key in the credentials secret is still honoured when
`spec.endpoint` is unset, so older ProviderConfigs continue to work.

## What's new in v0.3.x

The 0.3 line replaces the upjet-generated `Team` and `Membership` resources
with hand-written controllers. The upjet/Terraform shape can't drive
Gitea's per-unit permission model (it serialises `units` as a single
string and ignores `repo.packages` / `repo.actions`), and `Membership`
needed by-name team lookup so claims don't have to reach across XRs to
find a numeric team ID.

Two ergonomic improvements come out of the rewrite:

- **`Team.spec.forProvider.unitsMap`** — a per-unit permission map
  (e.g. `repo.code: write`, `repo.packages: read`) instead of a single
  permission level applied to a list of unit names.
- **`Membership.spec.forProvider.team`** — reference a team by
  `{org, name}` instead of by numeric ID. The controller resolves the ID
  at reconcile time and caches it in `status.atProvider.teamId`.

Both shapes are **strictly back-compatible** with the upstream v0.2.x
upjet shape:

- `Team` accepts either the new `unitsMap` *or* the legacy
  `permission` (+ optional string-encoded `units`) pair. When both are
  set, `unitsMap` wins.
- `Membership` accepts either the new `team {org, name}` ref *or* the
  legacy numeric `teamId`. When both are set, `team` wins.

Existing v0.2.x managed resources keep reconciling unchanged after the
upgrade — no `kubectl apply` migration required. Cluster-scoped Team and
Membership MRs that already hold string-encoded numeric IDs (the
Terraform-state convention) continue to deserialise cleanly because
`status.atProvider.id` and friends remain `*string`.

## API groups

| API group | Resources | Notes |
|-----------|-----------|-------|
| `gitea.gitea.crossplane.io` | `Org`, `Repository`, `Team`, `Token`, `User`, `Fork` | `Team` is hand-written |
| `repository.gitea.crossplane.io` | `ActionsSecret`, `ActionsVariable`, `BranchProtection`, `Key`, `OrgActionsSecret`, `UserActionsSecret`, `Webhook` | `*ActionsSecret` are hand-written |
| `team.gitea.crossplane.io` | `Members`, `Membership` | `Membership` is hand-written |
| `git.gitea.crossplane.io` | `Hook` | |
| `gpg.gitea.crossplane.io` | `Key` | |
| `oauth2.gitea.crossplane.io` | `App` | |
| `public.gitea.crossplane.io` | `Key` | |
| `gitea.crossplane.io` | `ProviderConfig`, `ProviderConfigUsage` | |

Namespaced equivalents use the `*.gitea.m.crossplane.io` suffix
(e.g. `gitea.gitea.m.crossplane.io/Team`).

## Resources

### Org

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: Org
metadata:
  name: my-org
spec:
  forProvider:
    name: my-org
    description: "My organisation"
    visibility: public
```

### Repository

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: Repository
metadata:
  name: my-repo
spec:
  forProvider:
    name: my-repo
    username: my-org    # user or org owner
    private: true
    autoInit: true
    defaultBranch: main
```

### Team

Hand-written. Express permissions via `unitsMap` (preferred) or the legacy
`permission` + `units` pair. When both are set, `unitsMap` wins.

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: Team
metadata:
  name: developers
spec:
  forProvider:
    name: developers
    organisation: my-org
    description: "Development team"
    includeAllRepositories: true
    canCreateOrgRepo: false
    unitsMap:
      repo.code:     write
      repo.issues:   write
      repo.pulls:    write
      repo.releases: write
      repo.packages: read
      repo.actions:  read
      repo.wiki:     read
      repo.projects: none
      repo.ext_issues: none
      repo.ext_wiki:   none
```

Valid unit keys: `repo.code`, `repo.issues`, `repo.ext_issues`,
`repo.wiki`, `repo.ext_wiki`, `repo.pulls`, `repo.releases`,
`repo.projects`, `repo.packages`, `repo.actions`. A unit absent from the
map is `none`. Levels: `none`, `read`, `write`, `admin`.

Legacy form (still accepted for back-compat with v0.2.x MRs):

```yaml
spec:
  forProvider:
    name: developers
    organisation: my-org
    permission: write
    units: "[repo.code, repo.pulls, repo.issues]"
```

### Membership

Hand-written. Reference the team by `{org, name}` (preferred) or by
numeric `teamId` (legacy).

```yaml
apiVersion: team.gitea.crossplane.io/v1alpha1
kind: Membership
metadata:
  name: johndoe-in-developers
spec:
  forProvider:
    team:
      org: my-org
      name: developers
    username: johndoe
```

The numeric team ID is resolved from `org`+`name` on first reconcile and
cached in `status.atProvider.teamId` to skip the lookup on subsequent
reconciles. The external-name annotation is `<teamID>:<username>`.

Legacy form:

```yaml
spec:
  forProvider:
    teamId: 42
    username: johndoe
```

### Members

Bulk team membership (upjet-generated):

```yaml
apiVersion: team.gitea.crossplane.io/v1alpha1
kind: Members
metadata:
  name: developers-members
spec:
  forProvider:
    teamIdRef:
      name: developers
    members:
      - johndoe
      - janedoe
```

### User

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: User
metadata:
  name: john-doe
spec:
  forProvider:
    username: johndoe
    loginName: johndoe
    email: john@example.com
    passwordSecretRef:
      name: user-password
      key: password
```

### Token

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: Token
metadata:
  name: ci-token
spec:
  forProvider:
    name: ci-token
    scopes:
      - write:repository
      - read:package
```

`Token` requires a username/password ProviderConfig — the Gitea endpoint
only accepts basic auth.

### Repository Key (deploy key)

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: Key
metadata:
  name: deploy-key
spec:
  forProvider:
    repositoryRef:
      name: my-repo
    title: "CI deploy key"
    keySecretRef:
      name: ssh-key
      key: public
    readOnly: false
```

### Webhook

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: Webhook
metadata:
  name: notify-webhook
spec:
  forProvider:
    repositoryRef:
      name: my-repo
    type: gitea
    events: [push, pull_request]
    url: https://example.com/webhook
    active: true
```

### BranchProtection

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: BranchProtection
metadata:
  name: main-protection
spec:
  forProvider:
    repositoryRef:
      name: my-repo
    ruleName: main
    requiredApprovals: 1
    requireSignedCommits: true
    dismissStaleApprovals: true
```

### ActionsSecret (repo-scoped)

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: ActionsSecret
metadata:
  name: registry-secret
spec:
  forProvider:
    repositoryOwner: my-org
    repository: my-repo
    secretName: REGISTRY_TOKEN
    secretValueSecretRef:
      name: registry-creds
      namespace: default
      key: token
```

### OrgActionsSecret

Org-scoped Actions secret, visible to workflows in any repo under the
organisation. The principal in the referenced ProviderConfig must have
org-owner rights (or be a site admin).

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: OrgActionsSecret
metadata:
  name: shared-deploy-key
spec:
  forProvider:
    org: my-org
    secretName: SHARED_DEPLOY_KEY
    secretValueSecretRef:
      name: deploy-key-source
      namespace: default
      key: token
```

The controller hashes the source secret value (SHA-256) on each
Create/Update and records it in `status.atProvider.valueHash`.
Subsequent Observe calls re-hash the current source value and compare;
if it has changed, the controller calls Update to push the new value to
Gitea. This is the only way to detect rotation, because Gitea's API
does not expose secret values for direct comparison.

### UserActionsSecret

User-scoped Actions secret on the authenticated user's account. Only
useful for workflows in repositories owned directly by that user.

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: UserActionsSecret
metadata:
  name: my-self-token
spec:
  forProvider:
    secretName: SELF_TOKEN
    secretValueSecretRef:
      name: self-token-source
      namespace: default
      key: token
```

The user is determined by the credentials of the referenced
ProviderConfig. Gitea's `/user/actions/secrets` endpoint operates on
the authenticated user and exposes neither GET nor LIST, so the
controller uses Crossplane's `crossplane.io/external-create-succeeded`
annotation as the existence marker. Value rotation is detected via the
same SHA-256 hash mechanism as `OrgActionsSecret`.

### Other resources

- `git.gitea.crossplane.io/Hook` — server-side git hooks
- `gpg.gitea.crossplane.io/Key` — GPG keys for commit signing
- `oauth2.gitea.crossplane.io/App` — OAuth2 applications
- `public.gitea.crossplane.io/Key` — SSH public keys for users

## Namespaced resources

Every resource above has a namespaced equivalent under the
`*.gitea.m.crossplane.io` API group, intended for compositions:

```yaml
apiVersion: gitea.gitea.m.crossplane.io/v1alpha1
kind: Team
metadata:
  name: developers
  namespace: platform
spec:
  forProvider:
    name: developers
    organisation: my-org
    unitsMap:
      repo.code: write
      repo.pulls: write
```

## Upgrading from v0.2.x

Upgrading is a provider package swap; no MR migration required. Read on
only if you want to migrate to the new shapes.

The hand-written `Team` and `Membership` controllers accept both the new
and legacy shapes:

| Resource | Legacy field (kept) | New field (preferred) | Precedence |
|----------|---------------------|-----------------------|------------|
| `Team` | `permission` (+ `units`) | `unitsMap` | `unitsMap` wins |
| `Membership` | `teamId` | `team {org, name}` | `team` wins |

If you choose to migrate an existing MR to the new shape, do it with a
plain `kubectl apply` — the controller observes the spec change and
reconciles. The legacy fields can be removed once you're on the new
shape. There is no flag day.

## Development

### Defaults

- Terraform provider source: `go-gitea/gitea`
- Terraform provider version: `0.7.0`
- CRD root group: `crossplane.io` (override with `CRD_ROOT_GROUP`)

### Generation (Podman)

Run the upjet generator in a Linux container similar to GitHub Actions.
Writes generated files into the working tree.

```bash
OWNER=<github-user> ./hack/generate.sh
```

Optional overrides: `CRD_ROOT_GROUP`, `TERRAFORM_PROVIDER_VERSION`,
`IMAGE` (default `ubuntu:24.04`).

The hand-written `Team`, `Membership`, `OrgActionsSecret` and
`UserActionsSecret` types live alongside the generated tree and are not
regenerated by `hack/generate.sh`. Their CRDs are produced with
`controller-gen`:

```bash
controller-gen crd:allowDangerousTypes=true \
  paths="./apis/cluster/...;./apis/namespaced/..." \
  output:crd:dir=package/crds
```

### Tests (Podman)

```bash
./hack/test.sh
```

Or directly: `go test ./...`.

### Build package (Podman)

Requires a Docker or Podman socket. For Podman, ensure the service is
running. If no socket exists, start one with:

```bash
podman system service --time=0 unix://$HOME/.local/share/containers/podman/podman.sock &
```

On macOS you can also use the Podman machine socket:

```bash
podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}'
```

```bash
VERSION=0.3.2 ./hack/build.sh
```

### Publish

Tag a release (e.g. `v0.3.2`) to publish the provider package to GHCR:

```
ghcr.io/<github-user>/provider-upjet-gitea:<tag>
```
