#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

parse_common_args "test.sh" "Run unit tests in a container." "" "$@"

IMAGE="${IMAGE:-${DEFAULT_IMAGE}}"

TEST_SCRIPT=$(cat <<'TEST_EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS=/bin/true

log_step() { echo "==> $*"; }

log_step "Installing system packages..."
apt-get -qq update
apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates

log_step "Installing Go..."
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

TEST_EOF
)

TEST_SCRIPT+="curl -fsSL \"https://go.dev/dl/go${GO_VERSION}.linux-\${GOARCH}.tar.gz\" | tar -C /usr/local -xz"

TEST_SCRIPT+=$(cat <<'TEST_EOF'

export PATH=/usr/local/go/bin:$PATH
export PATH="$(go env GOPATH)/bin:$PATH"

log_step "Initializing git submodules..."
git submodule sync
git submodule update --init --recursive

mkdir -p .work
LOG_PATH=".work/test.log"

log_step "Running make test..."
if make test >"${LOG_PATH}" 2>&1; then
  echo "Tests completed successfully."
else
  echo "Tests failed. See ${LOG_PATH}." >&2
  tail -n 200 "${LOG_PATH}" >&2
  exit 1
fi
TEST_EOF
)

run_in_container() {
  local runtime="$1"
  
  local run_args=(
    --rm -it
    -v "${PWD}:/workspace"
    -w /workspace
    -e "GIT_TERMINAL_PROMPT=0"
    -e "GIT_ASKPASS=/bin/true"
  )
  
  run_args+=("${IMAGE}")
  
  "$runtime" run "${run_args[@]}" bash -lc "$TEST_SCRIPT"
}

RUNTIME="$(detect_container_runtime)"
log_info "Running tests with ${RUNTIME}..."
run_in_container "$RUNTIME"
