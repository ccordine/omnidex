#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

compose() {
  env -u DOCKER_CONTEXT -u DOCKER_CONFIG \
    docker --host unix:///var/run/docker.sock compose "$@"
}

compose pull --ignore-buildable --policy always
compose build --pull --no-cache
compose down --volumes --remove-orphans
compose up -d --remove-orphans --build --force-recreate --wait --wait-timeout 180
