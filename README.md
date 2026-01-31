# provider-upjet-gitea

Crossplane provider for Gitea, generated with Upjet.

## Workflows

### Generate provider code
Run the GitHub Actions workflow `Generate Provider` to bootstrap the repo from
the Upjet template, apply the Gitea Terraform provider settings, and generate
the provider code.

Defaults in the workflow:
- Terraform provider source: `go-gitea/gitea`
- Terraform provider version: `0.7.0`
- CRD root group: `crossplane.io`

Override the CRD root group with a repo variable:
- `CRD_ROOT_GROUP`

### Publish provider package
Tag a release (e.g. `v0.1.0`) to publish the provider package to GHCR:

```
ghcr.io/<github-user>/provider-upjet-gitea:<tag>
```
