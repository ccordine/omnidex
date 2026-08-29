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

system_docker=(sudo env -u DOCKER_CONFIG -u DOCKER_CONTEXT -u DOCKER_HOST \
  -u DOCKER_CERT_PATH -u DOCKER_TLS -u DOCKER_TLS_VERIFY \
  -u BUILDKIT_HOST -u BUILDKIT_TLS_SERVER_NAME -u BUILDKIT_TLS_CACERT \
  -u BUILDKIT_TLS_CERT -u BUILDKIT_TLS_KEY -u BUILDX_BUILDER -u BUILDX_CONFIG \
  docker --context default)

docker_endpoint="$("${system_docker[@]}" context inspect default --format '{{(index .Endpoints "docker").Host}}')"
[[ "${docker_endpoint}" == "unix:///var/run/docker.sock" ]] || {
  printf 'default Docker context is not the required rootful /var/run/docker.sock authority\n' >&2
  exit 1
}
docker_security="$("${system_docker[@]}" info --format '{{json .SecurityOptions}}')"
[[ "${docker_security}" == \[*\] && "${docker_security}" != *name=rootless* ]] || {
  printf 'default Docker daemon is not qualified rootful authority\n' >&2
  exit 1
}

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
