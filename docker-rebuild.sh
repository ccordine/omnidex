#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

die() {
  printf '[docker-rebuild][error] %s\n' "$*" >&2
  exit 1
}

source "${SCRIPT_DIR}/scripts/managed-checkout-lib.sh"
managed_checkout_export_build_commit "${SCRIPT_DIR}"

compose() {
  env -u DOCKER_CONTEXT -u DOCKER_CONFIG \
    docker --host unix:///var/run/docker.sock compose "$@"
}

compose pull --ignore-buildable --policy always
compose build --pull --no-cache
compose down --volumes --remove-orphans
compose up -d --remove-orphans --build --force-recreate --wait --wait-timeout 180
"${SCRIPT_DIR}/up.sh"
