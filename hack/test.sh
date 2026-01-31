#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-ubuntu:24.04}"

podman run --rm -it \
  -v "${PWD}:/workspace" \
  -w /workspace \
  -e GIT_TERMINAL_PROMPT=0 \
  -e GIT_ASKPASS=/bin/true \
  "${IMAGE}" \
  bash -lc '
    set -euo pipefail
    export GIT_TERMINAL_PROMPT=0
    export GIT_ASKPASS=/bin/true
    export DEBIAN_FRONTEND=noninteractive
    apt-get -qq update
    apt-get -qq install -y --no-install-recommends git make curl unzip ca-certificates

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
    git submodule sync
    git submodule update --init --recursive

    mkdir -p .work
    LOG_PATH=".work/test.log"

    echo "Running make test..."
    if make test >"${LOG_PATH}" 2>&1; then
      echo "Tests completed successfully."
    else
      echo "Tests failed. See ${LOG_PATH}." >&2
      tail -n 200 "${LOG_PATH}" >&2
      exit 1
    fi
  '
