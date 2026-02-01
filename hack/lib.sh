#!/usr/bin/env bash
# hack/lib.sh - Shared functions for hack scripts
#
# Usage: source this file at the top of other hack scripts
#   source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

set -euo pipefail

# =============================================================================
# Configuration
# =============================================================================

# Go version - single source of truth for all scripts
readonly GO_VERSION="${GO_VERSION:-1.24.0}"

# Default container image
readonly DEFAULT_IMAGE="ubuntu:24.04"

# =============================================================================
# Logging
# =============================================================================

_log_prefix() {
  echo "[hack]"
}

log_info() {
  echo "$(_log_prefix) $*"
}

log_error() {
  echo "$(_log_prefix) ERROR: $*" >&2
}

log_warn() {
  echo "$(_log_prefix) WARN: $*" >&2
}

# =============================================================================
# Input Validation
# =============================================================================

# Validate that a variable contains only safe characters (alphanumeric, dash, underscore, dot, slash)
validate_safe_string() {
  local name="$1"
  local value="$2"
  if [[ ! "$value" =~ ^[a-zA-Z0-9_./-]+$ ]]; then
    log_error "Invalid characters in ${name}: ${value}"
    exit 1
  fi
}

# =============================================================================
# Container Runtime Detection
# =============================================================================
#
# SECURITY NOTE: build.sh uses --privileged for Docker-in-Docker builds.
# This grants the container full access to the host's devices and disables
# security features like AppArmor/SELinux. Only use in trusted environments.
# For CI, consider using kaniko or buildah for rootless builds instead.

# Detect available container runtime (docker or podman)
# Returns: "docker", "podman", or exits with error
detect_container_runtime() {
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    echo "docker"
  elif command -v podman >/dev/null 2>&1; then
    echo "podman"
  else
    log_error "Neither Docker nor Podman found. Please install one."
    exit 1
  fi
}

# Check if running on macOS with Podman machine
is_macos_podman() {
  [[ "$(uname -s)" == "Darwin" ]] && command -v podman >/dev/null 2>&1
}

# Get Podman machine state (running, stopped, etc.)
get_podman_machine_state() {
  podman machine inspect --format '{{.State}}' 2>/dev/null || echo "not_found"
}

# Get Podman machine socket path dynamically
get_podman_machine_socket() {
  podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null || echo ""
}

# Find available container socket
find_container_socket() {
  local docker_socket="/var/run/docker.sock"
  local user_podman_socket="/run/user/$(id -u)/podman/podman.sock"
  
  if [[ -S "$docker_socket" ]]; then
    echo "$docker_socket"
  elif [[ -S "$user_podman_socket" ]]; then
    echo "$user_podman_socket"
  else
    echo ""
  fi
}

# =============================================================================
# Container Build Helpers (for use inside containers)
# =============================================================================

# Generate the inline script for container setup
# This is meant to be eval'd inside the container
generate_container_setup_script() {
  local install_docker="${1:-false}"
  local install_buildx="${2:-false}"
  
  local docker_packages=""
  if [[ "$install_docker" == "true" ]]; then
    docker_packages="docker.io"
    if [[ "$install_buildx" == "true" ]]; then
      docker_packages="docker.io docker-buildx"
    fi
  fi

  cat <<'SETUP_EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS=/bin/true

log_step() { echo "==> $*"; }

log_step "Installing system packages..."
apt-get -qq update
SETUP_EOF

  if [[ -n "$docker_packages" ]]; then
    echo "apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates ${docker_packages}"
  else
    echo "apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates"
  fi

  cat <<SETUP_EOF

log_step "Installing Go ${GO_VERSION}..."
ARCH="\$(uname -m)"
case "\${ARCH}" in
  x86_64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "Unsupported arch: \${ARCH}" >&2; exit 1 ;;
esac

curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-\${GOARCH}.tar.gz" | tar -C /usr/local -xz
export PATH=/usr/local/go/bin:\$PATH
export PATH="\$(go env GOPATH)/bin:\$PATH"

log_step "Configuring git..."
git config --global --add safe.directory /workspace

log_step "Initializing git submodules..."
git submodule sync
git submodule update --init --recursive
SETUP_EOF
}

# =============================================================================
# Script Helpers
# =============================================================================

show_help() {
  local script_name="$1"
  local description="$2"
  local options="${3:-}"
  
  cat <<EOF
Usage: ${script_name} [OPTIONS]

${description}

Options:
  -h, --help    Show this help message and exit
${options}
Environment Variables:
  IMAGE         Container image to use (default: ${DEFAULT_IMAGE})
  GO_VERSION    Go version to install (default: ${GO_VERSION})

EOF
}

# Parse common arguments (--help)
parse_common_args() {
  local script_name="$1"
  local description="$2"
  local options="${3:-}"
  shift 3
  
  for arg in "$@"; do
    case "$arg" in
      -h|--help)
        show_help "$script_name" "$description" "$options"
        exit 0
        ;;
    esac
  done
}
