#!/usr/bin/env bash
set -euo pipefail

WORKDIR="$(pwd)"

TEMPLATE_REPO="${TEMPLATE_REPO:-https://github.com/crossplane/upjet-provider-template.git}"

PROVIDER_NAME_LOWER="${PROVIDER_NAME_LOWER:-gitea}"
PROVIDER_NAME_NORMAL="${PROVIDER_NAME_NORMAL:-Gitea}"
PROJECT_NAME="${PROJECT_NAME:-provider-upjet-gitea}"
CRD_ROOT_GROUP="${CRD_ROOT_GROUP:-crossplane.io}"

OWNER="${OWNER:-${GITHUB_REPOSITORY_OWNER:-}}"
if [ -z "${OWNER}" ]; then
  echo "OWNER is required (set OWNER or GITHUB_REPOSITORY_OWNER)." >&2
  exit 1
fi

TERRAFORM_PROVIDER_SOURCE="${TERRAFORM_PROVIDER_SOURCE:-go-gitea/gitea}"
TERRAFORM_PROVIDER_REPO="${TERRAFORM_PROVIDER_REPO:-https://github.com/go-gitea/terraform-provider-gitea}"
TERRAFORM_PROVIDER_VERSION="${TERRAFORM_PROVIDER_VERSION:-0.7.0}"
TERRAFORM_PROVIDER_DOWNLOAD_NAME="${TERRAFORM_PROVIDER_DOWNLOAD_NAME:-terraform-provider-gitea}"
TERRAFORM_NATIVE_PROVIDER_BINARY="${TERRAFORM_NATIVE_PROVIDER_BINARY:-terraform-provider-gitea_v${TERRAFORM_PROVIDER_VERSION}}"
TERRAFORM_DOCS_PATH="${TERRAFORM_DOCS_PATH:-docs}"

bootstrap_from_template() {
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  git clone --depth 1 "${TEMPLATE_REPO}" "${tmp_dir}"
  rsync -a --delete \
    --exclude ".git" \
    --exclude ".github/workflows" \
    --exclude "hack/bootstrap.sh" \
    --exclude "README.md" \
    "${tmp_dir}/" "${WORKDIR}/"
  rm -rf "${tmp_dir}"

  PROVIDER_NAME_LOWER="${PROVIDER_NAME_LOWER}" \
  PROVIDER_NAME_NORMAL="${PROVIDER_NAME_NORMAL}" \
  ORGANIZATION_NAME="${OWNER}" \
  CRD_ROOT_GROUP="${CRD_ROOT_GROUP}" \
  ./hack/prepare.sh
}

if [ ! -f "${WORKDIR}/cmd/generator/main.go" ]; then
  bootstrap_from_template
fi

if [ "${PROJECT_NAME}" != "provider-${PROVIDER_NAME_LOWER}" ]; then
  if [ -d "${WORKDIR}/cluster/images/provider-${PROVIDER_NAME_LOWER}" ]; then
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

sed -i.bak "s|modulePath     = \"github.com/${OWNER}/provider-${PROVIDER_NAME_LOWER}\"|modulePath     = \"github.com/${OWNER}/${PROJECT_NAME}\"|" config/provider.go
rm -f config/provider.go.bak

if [ "${GENERATE:-false}" = "true" ]; then
  make submodules
  make generate
fi
