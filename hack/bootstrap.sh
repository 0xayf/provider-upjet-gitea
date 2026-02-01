#!/usr/bin/env bash
set -euo pipefail

WORKDIR="$(pwd)"
export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS=/bin/true

TEMPLATE_TARBALL_URL="${TEMPLATE_TARBALL_URL:-https://codeload.github.com/crossplane/upjet-provider-template/tar.gz/refs/heads/main}"

PROVIDER_NAME_LOWER="${PROVIDER_NAME_LOWER:-gitea}"
PROVIDER_NAME_NORMAL="${PROVIDER_NAME_NORMAL:-Gitea}"
PROJECT_NAME="${PROJECT_NAME:-provider-upjet-gitea}"
CRD_ROOT_GROUP="${CRD_ROOT_GROUP:-crossplane.io}"

OWNER="${OWNER:-${GITHUB_REPOSITORY_OWNER:-}}"
if [[ -z "${OWNER}" ]]; then
  echo "OWNER is required (set OWNER or GITHUB_REPOSITORY_OWNER)." >&2
  exit 1
fi

TERRAFORM_PROVIDER_SOURCE="${TERRAFORM_PROVIDER_SOURCE:-go-gitea/gitea}"
TERRAFORM_PROVIDER_REPO="${TERRAFORM_PROVIDER_REPO:-https://github.com/go-gitea/terraform-provider-gitea}"
TERRAFORM_PROVIDER_VERSION="${TERRAFORM_PROVIDER_VERSION:-0.7.0}"
TERRAFORM_PROVIDER_DOWNLOAD_NAME="${TERRAFORM_PROVIDER_DOWNLOAD_NAME:-terraform-provider-gitea}"
TERRAFORM_NATIVE_PROVIDER_BINARY="${TERRAFORM_NATIVE_PROVIDER_BINARY:-terraform-provider-gitea_v${TERRAFORM_PROVIDER_VERSION}}"
TERRAFORM_DOCS_PATH="${TERRAFORM_DOCS_PATH:-docs}"

replace_in_files() {
  local pattern="$1"
  local replacement="$2"
  
  set +o pipefail
  grep -rl --exclude-dir=.git "$pattern" . 2>/dev/null | \
    xargs -r sed -i.bak "s|${pattern}|${replacement}|g" 2>/dev/null || true
  set -o pipefail
  find . -name "*.bak" -type f -delete 2>/dev/null || true
}

bootstrap_from_template() {
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  curl -fsSL "${TEMPLATE_TARBALL_URL}" | \
    tar -xz -C "${tmp_dir}" --strip-components=1
  
  rsync -a --delete \
    --exclude ".git" \
    --exclude ".cache" \
    --exclude ".work" \
    --exclude "build" \
    --exclude "examples" \
    --exclude "examples-generated" \
    --exclude "extensions" \
    --exclude ".github/workflows" \
    --exclude ".github/PULL_REQUEST_TEMPLATE.md" \
    --exclude ".github/renovate.json5" \
    --exclude ".golangci.yml" \
    --exclude "CODEOWNERS" \
    --exclude "CODE_OF_CONDUCT.md" \
    --exclude "LICENSE" \
    --exclude "OWNERS.md" \
    --exclude "hack/bootstrap.sh" \
    --exclude "hack/build.sh" \
    --exclude "hack/generate.sh" \
    --exclude "hack/lib.sh" \
    --exclude "hack/test.sh" \
    --exclude "README.md" \
    "${tmp_dir}/" "${WORKDIR}/"
  rm -rf "${tmp_dir}"

  rm -f "${WORKDIR}/.github/PULL_REQUEST_TEMPLATE.md" \
        "${WORKDIR}/.github/renovate.json5" \
        "${WORKDIR}/.golangci.yml" \
        "${WORKDIR}/CODEOWNERS" \
        "${WORKDIR}/CODE_OF_CONDUCT.md" \
        "${WORKDIR}/LICENSE" \
        "${WORKDIR}/OWNERS.md"

  if [[ ! -f "${WORKDIR}/hack/prepare.sh" ]]; then
    curl -fsSL "https://raw.githubusercontent.com/crossplane/upjet-provider-template/main/hack/prepare.sh" \
      -o "${WORKDIR}/hack/prepare.sh"
  fi

  if [[ -f "${WORKDIR}/hack/prepare.sh" ]]; then
    sed -i.bak 's/set -euox pipefail/set -uox pipefail/' "${WORKDIR}/hack/prepare.sh"
    sed -i.bak 's/| xargs /| xargs -r /g' "${WORKDIR}/hack/prepare.sh"
    sed -i.bak 's/:!hack\/prepare\.sh/:!hack\/prepare\.sh :!hack\/bootstrap\.sh :!hack\/build\.sh :!hack\/generate\.sh :!hack\/lib\.sh :!hack\/test\.sh/' "${WORKDIR}/hack/prepare.sh"
    sed -i.bak 's/git clean -fd/find . -name "*.bak" -type f -delete/' "${WORKDIR}/hack/prepare.sh"
    sed -i.bak 's/^git mv /mv /' "${WORKDIR}/hack/prepare.sh"
    rm -f "${WORKDIR}/hack/prepare.sh.bak"
    chmod +x "${WORKDIR}/hack/prepare.sh"
  fi

  PROVIDER_NAME_LOWER="${PROVIDER_NAME_LOWER}" \
  PROVIDER_NAME_NORMAL="${PROVIDER_NAME_NORMAL}" \
  ORGANIZATION_NAME="${OWNER}" \
  CRD_ROOT_GROUP="${CRD_ROOT_GROUP}" \
  ./hack/prepare.sh

  if [[ -f "${WORKDIR}/config/provider.go" ]]; then
    sed -i.bak '/config\/cluster\/null/d' "${WORKDIR}/config/provider.go"
    sed -i.bak '/config\/namespaced\/null/d' "${WORKDIR}/config/provider.go"
    sed -i.bak '/nullCluster\.Configure/d' "${WORKDIR}/config/provider.go"
    sed -i.bak '/nullNamespaced\.Configure/d' "${WORKDIR}/config/provider.go"
    rm -f "${WORKDIR}/config/provider.go.bak"
  fi
}

apply_project_replacements() {
  replace_in_files "github.com/${OWNER}/provider-${PROVIDER_NAME_LOWER}" "github.com/${OWNER}/${PROJECT_NAME}"
  replace_in_files "${OWNER}/provider-${PROVIDER_NAME_LOWER}" "${OWNER}/${PROJECT_NAME}"
  replace_in_files "github.com/0xayf/provider-upjet-gitea" "github.com/${OWNER}/${PROJECT_NAME}"
  replace_in_files "provider-upjet-gitea" "${PROJECT_NAME}"
}

if [[ "${FORCE_BOOTSTRAP:-}" == "1" ]] || [[ ! -f "${WORKDIR}/cmd/generator/main.go" ]] || [[ ! -d "${WORKDIR}/cluster/images/${PROJECT_NAME}" ]]; then
  bootstrap_from_template
fi

apply_project_replacements

if [[ "${PROJECT_NAME}" != "provider-${PROVIDER_NAME_LOWER}" ]]; then
  if [[ -d "${WORKDIR}/cluster/images/provider-${PROVIDER_NAME_LOWER}" ]]; then
    mv "${WORKDIR}/cluster/images/provider-${PROVIDER_NAME_LOWER}" "${WORKDIR}/cluster/images/${PROJECT_NAME}"
  fi
fi

sed -i.bak "s|^PROJECT_NAME ?= .*|PROJECT_NAME ?= ${PROJECT_NAME}|" Makefile
sed -i.bak "s|^PROJECT_REPO ?= .*|PROJECT_REPO ?= github.com/${OWNER}/${PROJECT_NAME}|" Makefile
sed -i.bak "s|^export TERRAFORM_PROVIDER_SOURCE ?= .*|export TERRAFORM_PROVIDER_SOURCE ?= ${TERRAFORM_PROVIDER_SOURCE}|" Makefile
sed -i.bak "s|^export TERRAFORM_PROVIDER_REPO ?= .*|export TERRAFORM_PROVIDER_REPO ?= ${TERRAFORM_PROVIDER_REPO}|" Makefile
sed -i.bak "s|^export TERRAFORM_PROVIDER_VERSION ?= .*|export TERRAFORM_PROVIDER_VERSION ?= ${TERRAFORM_PROVIDER_VERSION}|" Makefile
sed -i.bak "s|^export TERRAFORM_PROVIDER_DOWNLOAD_NAME ?= .*|export TERRAFORM_PROVIDER_DOWNLOAD_NAME ?= ${TERRAFORM_PROVIDER_DOWNLOAD_NAME}|" Makefile
sed -i.bak "s|^export TERRAFORM_NATIVE_PROVIDER_BINARY ?= .*|export TERRAFORM_NATIVE_PROVIDER_BINARY ?= ${TERRAFORM_NATIVE_PROVIDER_BINARY}|" Makefile
sed -i.bak "s|^export TERRAFORM_DOCS_PATH ?= .*|export TERRAFORM_DOCS_PATH ?= ${TERRAFORM_DOCS_PATH}|" Makefile
sed -i.bak "s|^REGISTRY_ORGS ?= .*|REGISTRY_ORGS ?= ghcr.io/${OWNER}|" Makefile
sed -i.bak "s|^XPKG_REG_ORGS ?= .*|XPKG_REG_ORGS ?= ghcr.io/${OWNER}|" Makefile
sed -i.bak "s|^XPKG_REG_ORGS_NO_PROMOTE ?= .*|XPKG_REG_ORGS_NO_PROMOTE ?= ghcr.io/${OWNER}|" Makefile
rm -f Makefile.bak

go mod edit -module "github.com/${OWNER}/${PROJECT_NAME}"

sed -i.bak "s|^modulePath     = \".*\"|modulePath     = \"github.com/${OWNER}/${PROJECT_NAME}\"|" config/provider.go
rm -f config/provider.go.bak

if [[ "${GENERATE:-false}" == "true" ]]; then
  make submodules
  make generate
fi
