# provider-upjet-gitea

Crossplane provider for Gitea, generated with Upjet.

## Defaults

Generation defaults:
- Terraform provider source: `go-gitea/gitea`
- Terraform provider version: `0.7.0`
- CRD root group: `crossplane.io`

Override the CRD root group with:
- `CRD_ROOT_GROUP`

## Generation (Podman)
Run the generator in a Linux container similar to GitHub Actions. This writes
generated files into your working tree.

```bash
OWNER=<github-user> ./hack/generate.sh
```

Optional overrides:
- `CRD_ROOT_GROUP`
- `TERRAFORM_PROVIDER_VERSION`
- `IMAGE` (default `ubuntu:24.04`)

## Tests (Podman)
Run unit tests in a Linux container after generating files.

```bash
./hack/test.sh
```

## Publish provider package
Tag a release (e.g. `v0.1.0`) to publish the provider package to GHCR:

```
ghcr.io/<github-user>/provider-upjet-gitea:<tag>
```
