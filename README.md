# provider-upjet-gitea

Crossplane provider for Gitea, generated with Upjet.

## API Reference

This provider exposes 39 CRDs across 8 API groups, supporting both cluster-scoped and namespaced resources.

### API Groups

| API Group | Resources | Scope |
|-----------|-----------|-------|
| `gitea.gitea.crossplane.io` | Org, Repository, Team, Token, User, Fork | Cluster |
| `repository.gitea.crossplane.io` | ActionsSecret, ActionsVariable, BranchProtection, Key, Webhook | Cluster |
| `team.gitea.crossplane.io` | Members, Membership | Cluster |
| `git.gitea.crossplane.io` | Hook | Cluster |
| `gpg.gitea.crossplane.io` | Key | Cluster |
| `oauth2.gitea.crossplane.io` | App | Cluster |
| `public.gitea.crossplane.io` | Key | Cluster |
| `gitea.crossplane.io` | ProviderConfig, ProviderConfigUsage | Cluster |

Namespaced variants use `.gitea.m.crossplane.io` suffix (e.g., `gitea.gitea.m.crossplane.io`).

### Core Resources (`gitea.gitea.crossplane.io`)

#### Org
Manage Gitea organizations for grouping repositories and teams.

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: Org
metadata:
  name: my-org
spec:
  forProvider:
    name: my-org
    description: "My organization"
    visibility: public
```

#### Repository
Create and manage Gitea repositories.

```yaml
apiVersion: gitea.gitea.crossplane.io/v1alpha1
kind: Repository
metadata:
  name: my-repo
spec:
  forProvider:
    name: my-repo
    username: my-org  # Can be user or org name
    private: true
    autoInit: true
    defaultBranch: main
```

#### Team
Manage teams within organizations for access control.

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
    permission: write
    includeAllRepositories: false
```

#### User
Create and manage Gitea user accounts.

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

#### Token
Create access tokens for API authentication.

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

### Repository Resources (`repository.gitea.crossplane.io`)

#### Repository Key
Deploy keys for repository access.

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: Key
metadata:
  name: deploy-key
spec:
  forProvider:
    repositoryRef:
      name: my-repo
    title: "CI Deploy Key"
    keySecretRef:
      name: ssh-key
      key: public
    readOnly: false
```

#### Webhook
Configure repository webhooks.

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
    events:
      - push
      - pull_request
    url: https://example.com/webhook
    active: true
```

#### BranchProtection
Protect branches with merge requirements.

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: BranchProtection
metadata:
  name: main-protection
spec:
  forProvider:
    repositoryRef:
      name: my-repo
    ruleName: "main"
    requiredApprovals: 1
    requireSignedCommits: true
    dismissStaleApprovals: true
```

#### ActionsSecret
Repository-level secrets for CI/CD.

```yaml
apiVersion: repository.gitea.crossplane.io/v1alpha1
kind: ActionsSecret
metadata:
  name: registry-secret
spec:
  forProvider:
    repositoryRef:
      name: my-repo
    secretName: REGISTRY_TOKEN
    secretValueSecretRef:
      name: registry-creds
      key: token
```

### Team Resources (`team.gitea.crossplane.io`)

#### Membership
Add users to teams.

```yaml
apiVersion: team.gitea.crossplane.io/v1alpha1
kind: Membership
metadata:
  name: dev-team-john
spec:
  forProvider:
    teamIdRef:
      name: developers
    username: johndoe
```

#### Members
Manage all team members at once.

```yaml
apiVersion: team.gitea.crossplane.io/v1alpha1
kind: Members
metadata:
  name: dev-team-members
spec:
  forProvider:
    teamIdRef:
      name: developers
    members:
      - johndoe
      - janedoe
```

### Other Resources

- **Git Hook** (`git.gitea.crossplane.io/Hook`) - Server-side git hooks
- **GPG Key** (`gpg.gitea.crossplane.io/Key`) - GPG keys for commit signing
- **OAuth2 App** (`oauth2.gitea.crossplane.io/App`) - OAuth2 applications
- **Public Key** (`public.gitea.crossplane.io/Key`) - SSH public keys for users

### Namespaced Resources

All resources above have namespaced equivalents using the `gitea.m.crossplane.io` API group suffix. Namespaced resources are scoped to a specific Kubernetes namespace and can be used in compositions.

Example:
```yaml
apiVersion: gitea.gitea.m.crossplane.io/v1alpha1
kind: Org
metadata:
  name: my-org
  namespace: default
spec:
  forProvider:
    name: my-org
```

## Provider Configuration

Create a ProviderConfig to authenticate with your Gitea instance:

```yaml
apiVersion: gitea.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: gitea-provider-config
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: gitea-credentials
      key: credentials
```

The secret should contain:
```json
{
  "base_url": "https://gitea.example.com",
  "token": "your-access-token"
}
```

## Development

### Defaults

Generation defaults:
- Terraform provider source: `go-gitea/gitea`
- Terraform provider version: `0.7.0`
- CRD root group: `crossplane.io`

Override the CRD root group with:
- `CRD_ROOT_GROUP`

### Generation (Podman)
Run the generator in a Linux container similar to GitHub Actions. This writes
generated files into your working tree.

```bash
OWNER=<github-user> ./hack/generate.sh
```

Optional overrides:
- `CRD_ROOT_GROUP`
- `TERRAFORM_PROVIDER_VERSION`
- `IMAGE` (default `ubuntu:24.04`)

### Tests (Podman)
Run unit tests in a Linux container after generating files.

```bash
./hack/test.sh
```

### Build package (Podman)
Build the provider image and xpkg in a Linux container. Requires a Docker or
Podman socket. For Podman, ensure the service is running. If no socket exists,
start one with:

```bash
podman system service --time=0 unix://$HOME/.local/share/containers/podman/podman.sock &
```

On macOS, you can also use the Podman machine socket:

```bash
podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}'
```

```bash
VERSION=0.1.0 ./hack/build.sh
```

## Publish provider package
Tag a release (e.g. `v0.1.0`) to publish the provider package to GHCR:

```
ghcr.io/<github-user>/provider-upjet-gitea:<tag>
```
