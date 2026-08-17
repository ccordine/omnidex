#!/usr/bin/env bash
# Restore the host-owned WorkNet edge/DNS runtime, then publish the installed
# Omnidex core on that same system Docker daemon.
set -euo pipefail

WORKNET_ROOT="${WORKNET_ROOT:-${HOME}/Networking}"
OMNIDEX_ROOT="${OMNIDEX_ROOT:-${HOME}/.omnidex}"

for required_path in \
  "${WORKNET_ROOT}/worknet-up.sh" \
  "${OMNIDEX_ROOT}/docker-compose.yml" \
  "${OMNIDEX_ROOT}/.env"; do
  if [[ ! -e "${required_path}" ]]; then
    printf 'missing required path: %s\n' "${required_path}" >&2
    exit 1
  fi
done

sudo -v

system_docker=(sudo env -u DOCKER_CONFIG -u DOCKER_CONTEXT -u DOCKER_HOST docker)

if ! "${system_docker[@]}" network inspect dev-net >/dev/null 2>&1; then
  "${system_docker[@]}" network create --driver bridge dev-net
fi

"${WORKNET_ROOT}/worknet-up.sh" --skip-deps --with-edge

"${system_docker[@]}" compose \
  -f "${OMNIDEX_ROOT}/docker-compose.yml" \
  up -d --remove-orphans --build --wait --wait-timeout 120

resolvectl query admin.worknet omni.worknet
curl -kfsS --max-time 20 https://admin.worknet/health >/dev/null
curl -kfsS --max-time 20 https://omni.worknet/readyz >/dev/null

printf 'WorkNet edge/DNS and Omnidex are ready.\n'
