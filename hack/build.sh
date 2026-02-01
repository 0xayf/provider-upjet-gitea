#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

parse_common_args "build.sh" "Build the Crossplane provider xpkg in a container." \
  "  VERSION     Package version (default: from git tag)" "$@"

IMAGE="${IMAGE:-${DEFAULT_IMAGE}}"

if [[ -n "${OWNER:-}" ]]; then
  validate_safe_string "OWNER" "$OWNER"
fi
if [[ -n "${VERSION:-}" ]]; then
  validate_safe_string "VERSION" "$VERSION"
fi

BUILD_SCRIPT=$(cat <<'BUILD_EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS=/bin/true

log_step() { echo "==> $*"; }

log_step "Installing system packages..."
apt-get -qq update
apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates docker.io docker-buildx

log_step "Installing Go..."
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

BUILD_EOF
)

BUILD_SCRIPT+="curl -fsSL \"https://go.dev/dl/go${GO_VERSION}.linux-\${GOARCH}.tar.gz\" | tar -C /usr/local -xz"

BUILD_SCRIPT+=$(cat <<'BUILD_EOF'

export PATH=/usr/local/go/bin:$PATH
export PATH="$(go env GOPATH)/bin:$PATH"

log_step "Configuring git..."
git config --global --add safe.directory /workspace

log_step "Initializing git submodules..."
git submodule sync
git submodule update --init --recursive

log_step "Building xpkg..."
make xpkg.build

echo "Build completed successfully."
BUILD_EOF
)

run_in_container() {
  local runtime="$1"
  local socket="$2"
  
  local run_args=(
    --rm -i
    --privileged
    -v "${PWD}:/workspace"
    -w /workspace
    -e GIT_TERMINAL_PROMPT=0
    -e GIT_ASKPASS=/bin/true
  )
  
  if [[ -n "$socket" ]]; then
    run_args+=(-v "${socket}:/var/run/docker.sock")
  fi
  
  if [[ -n "${VERSION:-}" ]]; then
    run_args+=(-e "VERSION=${VERSION}")
  fi
  
  run_args+=("${IMAGE}")
  
  "$runtime" run "${run_args[@]}" bash -c "$BUILD_SCRIPT"
}

if is_macos_podman; then
  MACHINE_STATE="$(get_podman_machine_state)"
  if [[ "$MACHINE_STATE" == "running" ]]; then
    SOCKET="$(get_podman_machine_socket)"
    if [[ -z "$SOCKET" ]]; then
      log_error "Could not determine Podman machine socket path."
      exit 1
    fi
    log_info "Building in Podman machine (socket: ${SOCKET})..."
    run_in_container "podman" "$SOCKET"
    exit 0
  else
    log_error "Podman machine is not running. Start it with: podman machine start"
    exit 1
  fi
fi

SOCKET="$(find_container_socket)"
if [[ -z "$SOCKET" ]]; then
  log_error "No container socket found."
  log_error "Start Docker or run: systemctl --user start podman.socket"
  exit 1
fi

RUNTIME="$(detect_container_runtime)"
log_info "Building with ${RUNTIME} (socket: ${SOCKET})..."
run_in_container "$RUNTIME" "$SOCKET"
