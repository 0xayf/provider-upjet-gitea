#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-ubuntu:24.04}"
OWNER="${OWNER:-}"

if [ -z "${OWNER}" ]; then
  echo "OWNER is required (set OWNER to your GitHub org/user)." >&2
  exit 1
fi

podman run --rm -it \
  -v "${PWD}:/workspace" \
  -w /workspace \
  -e OWNER="${OWNER}" \
  -e GENERATE="false" \
  -e GIT_TERMINAL_PROMPT=0 \
  -e GIT_ASKPASS=/bin/true \
  -e CRD_ROOT_GROUP="${CRD_ROOT_GROUP:-crossplane.io}" \
  -e TERRAFORM_PROVIDER_SOURCE="${TERRAFORM_PROVIDER_SOURCE:-go-gitea/gitea}" \
  -e TERRAFORM_PROVIDER_REPO="${TERRAFORM_PROVIDER_REPO:-https://github.com/go-gitea/terraform-provider-gitea}" \
  -e TERRAFORM_PROVIDER_VERSION="${TERRAFORM_PROVIDER_VERSION:-0.7.0}" \
  -e TERRAFORM_PROVIDER_DOWNLOAD_NAME="${TERRAFORM_PROVIDER_DOWNLOAD_NAME:-terraform-provider-gitea}" \
  -e TERRAFORM_DOCS_PATH="${TERRAFORM_DOCS_PATH:-docs}" \
  "${IMAGE}" \
  bash -lc '
    set -euo pipefail
    export GIT_TERMINAL_PROMPT=0
    export GIT_ASKPASS=/bin/true
    export DEBIAN_FRONTEND=noninteractive
    apt-get -qq update
    apt-get -qq install -y --no-install-recommends git make rsync curl unzip ca-certificates

    GO_VERSION=1.24.0
    ARCH="$(uname -m)"
    case "${ARCH}" in
      x86_64) GOARCH=amd64 ;;
      aarch64|arm64) GOARCH=arm64 ;;
      *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
    esac

    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" | tar -C /usr/local -xz
    export PATH=/usr/local/go/bin:$PATH

    go install golang.org/x/tools/cmd/goimports@latest
    export PATH="$(go env GOPATH)/bin:$PATH"

    echo "Running bootstrap..."
    chmod +x hack/bootstrap.sh
    ./hack/bootstrap.sh

    if [ -f config/provider.go ]; then
      sed -i.bak "/config\/cluster\/null/d" config/provider.go
      sed -i.bak "/config\/namespaced\/null/d" config/provider.go
      sed -i.bak "/nullCluster\.Configure/d" config/provider.go
      sed -i.bak "/nullNamespaced\.Configure/d" config/provider.go
      rm -f config/provider.go.bak
    fi

    echo "Initializing git submodules..."
    git submodule sync
    git submodule update --init --recursive

    mkdir -p .work
    LOG_PATH=".work/generate.log"

    echo "Running make generate..."
    if make generate >"${LOG_PATH}" 2>&1; then
      echo "Generate completed successfully."
    else
      echo "Generate failed. See ${LOG_PATH}." >&2
      tail -n 200 "${LOG_PATH}" >&2
      exit 1
    fi

    if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      git status -sb
      git diff --stat
    fi
  '
