#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

parse_common_args "generate.sh" "Run code generation in a container." \
  "  OWNER                        GitHub org/user (required)
  CRD_ROOT_GROUP               CRD root group (default: crossplane.io)
  TERRAFORM_PROVIDER_SOURCE    TF provider source (default: go-gitea/gitea)
  TERRAFORM_PROVIDER_VERSION   TF provider version (default: 0.7.0)" "$@"

IMAGE="${IMAGE:-${DEFAULT_IMAGE}}"
OWNER="${OWNER:-}"

if [[ -z "${OWNER}" ]]; then
  log_error "OWNER is required (set OWNER to your GitHub org/user)."
  exit 1
fi

validate_safe_string "OWNER" "$OWNER"

GENERATE_SCRIPT=$(cat <<'GENERATE_EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS=/bin/true

log_step() { echo "==> $*"; }

log_step "Installing system packages..."
apt-get -qq update
apt-get -qq install -y --no-install-recommends git make rsync curl unzip ca-certificates

log_step "Installing Go..."
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

GENERATE_EOF
)

GENERATE_SCRIPT+="curl -fsSL \"https://go.dev/dl/go${GO_VERSION}.linux-\${GOARCH}.tar.gz\" | tar -C /usr/local -xz"

GENERATE_SCRIPT+=$(cat <<'GENERATE_EOF'

export PATH=/usr/local/go/bin:$PATH
go install golang.org/x/tools/cmd/goimports@latest
export PATH="$(go env GOPATH)/bin:$PATH"

log_step "Running bootstrap..."
chmod +x hack/bootstrap.sh
./hack/bootstrap.sh

if [ -f config/provider.go ]; then
  sed -i.bak "/config\/cluster\/null/d" config/provider.go
  sed -i.bak "/config\/namespaced\/null/d" config/provider.go
  sed -i.bak "/nullCluster\.Configure/d" config/provider.go
  sed -i.bak "/nullNamespaced\.Configure/d" config/provider.go
  rm -f config/provider.go.bak
fi

log_step "Initializing git submodules..."
git submodule sync
git submodule update --init --recursive

log_step "Fetching Terraform docs..."
make pull-docs
if [ -d ".work/${TERRAFORM_PROVIDER_SOURCE}/.git" ]; then
  git -C ".work/${TERRAFORM_PROVIDER_SOURCE}" checkout --force HEAD -- "${TERRAFORM_DOCS_PATH}" || true
fi

mkdir -p .work
LOG_PATH=".work/generate.log"

log_step "Running make generate..."
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
GENERATE_EOF
)

run_in_container() {
  local runtime="$1"
  
  local run_args=(
    --rm -it
    -v "${PWD}:/workspace"
    -w /workspace
    -e "OWNER=${OWNER}"
    -e "GENERATE=false"
    -e "GIT_TERMINAL_PROMPT=0"
    -e "GIT_ASKPASS=/bin/true"
    -e "CRD_ROOT_GROUP=${CRD_ROOT_GROUP:-crossplane.io}"
    -e "TERRAFORM_PROVIDER_SOURCE=${TERRAFORM_PROVIDER_SOURCE:-go-gitea/gitea}"
    -e "TERRAFORM_PROVIDER_REPO=${TERRAFORM_PROVIDER_REPO:-https://github.com/go-gitea/terraform-provider-gitea}"
    -e "TERRAFORM_PROVIDER_VERSION=${TERRAFORM_PROVIDER_VERSION:-0.7.0}"
    -e "TERRAFORM_PROVIDER_DOWNLOAD_NAME=${TERRAFORM_PROVIDER_DOWNLOAD_NAME:-terraform-provider-gitea}"
    -e "TERRAFORM_DOCS_PATH=${TERRAFORM_DOCS_PATH:-docs}"
  )
  
  run_args+=("${IMAGE}")
  
  "$runtime" run "${run_args[@]}" bash -lc "$GENERATE_SCRIPT"
}

RUNTIME="$(detect_container_runtime)"
log_info "Running generation with ${RUNTIME}..."
run_in_container "$RUNTIME"
