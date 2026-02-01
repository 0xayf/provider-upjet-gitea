#!/usr/bin/env bash
set -euo pipefail

HOST_OS="$(uname -s)"
IMAGE="${IMAGE:-ubuntu:24.04}"

if [ "${HOST_OS}" = "Darwin" ] && command -v podman >/dev/null 2>&1; then
  MACHINE_STATE="$(podman machine inspect --format '{{.State}}' 2>/dev/null || true)"
  if [ "${MACHINE_STATE}" = "running" ]; then
    WORKDIR="$(pwd -P)"
    echo "Building in Podman machine..."
    
    RUN_ARGS=(
      --rm -i
      --privileged
      -v "${WORKDIR}:/workspace"
      -w /workspace
      -e GIT_TERMINAL_PROMPT=0
      -e GIT_ASKPASS=/bin/true
      -v /run/user/502/podman/podman.sock:/var/run/docker.sock
    )
    
    if [ -n "${VERSION:-}" ]; then
      RUN_ARGS+=(-e "VERSION=${VERSION}")
    fi
    
    RUN_ARGS+=("${IMAGE}")
    
    podman run "${RUN_ARGS[@]}" bash -c '
      set -euo pipefail
      export DEBIAN_FRONTEND=noninteractive
      export GIT_TERMINAL_PROMPT=0
      export GIT_ASKPASS=/bin/true

      apt-get -qq update
apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates docker.io docker-buildx

      GO_VERSION=1.24.0
      ARCH="$(uname -m)"
      case "${ARCH}" in
        x86_64) GOARCH=amd64 ;;
        aarch64|arm64) GOARCH=arm64 ;;
        *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
      esac

      curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" | tar -C /usr/local -xz
      export PATH=/usr/local/go/bin:$PATH
      export PATH="$(go env GOPATH)/bin:$PATH"

      echo "Initializing git submodules..."
      git config --global --add safe.directory /workspace
      git submodule sync
      git submodule update --init --recursive

      echo "Building xpkg..."
      make xpkg.build

      echo "Build completed successfully."
    '
    exit 0
  else
    echo "Podman machine is not running. Start it with: podman machine start" >&2
    exit 1
  fi
fi

DOCKER_SOCKET="/var/run/docker.sock"
PODMAN_SOCKET="${PODMAN_SOCKET:-/run/user/$(id -u)/podman/podman.sock}"

SOCKET=""
if [ -S "${DOCKER_SOCKET}" ]; then
  SOCKET="${DOCKER_SOCKET}"
elif [ -S "${PODMAN_SOCKET}" ]; then
  SOCKET="${PODMAN_SOCKET}"
fi

if [ -z "${SOCKET}" ]; then
  echo "Docker/Podman socket not available." >&2
  echo "Start Docker or run: systemctl --user start podman.socket" >&2
  exit 1
fi

RUN_ARGS=(
  --rm -it
  --privileged
  -v "${PWD}:/workspace"
  -w /workspace
  -e GIT_TERMINAL_PROMPT=0
  -e GIT_ASKPASS=/bin/true
  -v "${SOCKET}:/var/run/docker.sock"
)

if [ -n "${VERSION:-}" ]; then
  RUN_ARGS+=(-e "VERSION=${VERSION}")
fi

RUN_ARGS+=("${IMAGE}")

docker run "${RUN_ARGS[@]}" bash -c '
  set -euo pipefail
  export DEBIAN_FRONTEND=noninteractive
  export GIT_TERMINAL_PROMPT=0
  export GIT_ASKPASS=/bin/true

  apt-get -qq update
  apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates docker.io

  GO_VERSION=1.24.0
  ARCH="$(uname -m)"
  case "${ARCH}" in
    x86_64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
  esac

  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" | tar -C /usr/local -xz
  export PATH=/usr/local/go/bin:$PATH
  export PATH="$(go env GOPATH)/bin:$PATH"

  echo "Initializing git submodules..."
  git config --global --add safe.directory /workspace
  git submodule sync
  git submodule update --init --recursive

  echo "Building xpkg..."
  make xpkg.build

  echo "Build completed successfully."
' || podman run "${RUN_ARGS[@]}" bash -c '
  set -euo pipefail
  export DEBIAN_FRONTEND=noninteractive
  export GIT_TERMINAL_PROMPT=0
  export GIT_ASKPASS=/bin/true

  apt-get -qq update
  apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates docker.io

  GO_VERSION=1.24.0
  ARCH="$(uname -m)"
  case "${ARCH}" in
    x86_64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
  esac

  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" | tar -C /usr/local -xz
  export PATH=/usr/local/go/bin:$PATH
  export PATH="$(go env GOPATH)/bin:$PATH"

  echo "Initializing git submodules..."
  git config --global --add safe.directory /workspace
  git submodule sync
  git submodule update --init --recursive

  echo "Building xpkg..."
  make xpkg.build

  echo "Build completed successfully."
'
