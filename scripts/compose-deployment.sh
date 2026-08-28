#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

die() {
  printf '[compose][error] %s\n' "$*" >&2
  exit 1
}

log() {
  printf '[compose] %s\n' "$*"
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

source "${SCRIPT_DIR}/managed-checkout-lib.sh"
source "${SCRIPT_DIR}/update-runtime-lib.sh"

usage() {
  cat <<EOF
Usage: ./scripts/compose-deployment.sh <up|down> [--build]

Uses only the built-in default rootful Docker context and the exact
COMPOSE_PROJECT_NAME from .env. It never removes PostgreSQL or Redis volumes.
EOF
}

action="${1:-}"
case "${action}" in
  up|down)
    shift
    ;;
  -h|--help|"")
    usage
    exit 0
    ;;
  *)
    die "unsupported action ${action}; use up or down"
    ;;
esac

ENV_FILE="${REPO_DIR}/.env"
runtime_reject_managed_docker_routing_keys "${ENV_FILE}"
managed_checkout_require_env_key "${ENV_FILE}" "DOCKER_CONTEXT"
managed_checkout_require_env_key "${ENV_FILE}" "COMPOSE_PROJECT_NAME"
managed_checkout_require_env_key "${ENV_FILE}" "HOST_UID"
managed_checkout_require_env_key "${ENV_FILE}" "HOST_GID"
DOCKER_CONTEXT_NAME="$(managed_checkout_env_value "${ENV_FILE}" "DOCKER_CONTEXT")"
COMPOSE_PROJECT="$(managed_checkout_env_value "${ENV_FILE}" "COMPOSE_PROJECT_NAME")"
HOST_UID_VALUE="$(managed_checkout_env_value "${ENV_FILE}" "HOST_UID")"
HOST_GID_VALUE="$(managed_checkout_env_value "${ENV_FILE}" "HOST_GID")"
validate_compose_identity "COMPOSE_PROJECT_NAME" "${COMPOSE_PROJECT}"
runtime_require_rootful_docker_context
[[ -n "${COMPOSE_PROJECT}" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"
EXPECTED_RUNTIME_USER="$(runtime_user_identity "${HOST_UID_VALUE}" "${HOST_GID_VALUE}")"
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT}"
export HOST_UID="${HOST_UID_VALUE}"
export HOST_GID="${HOST_GID_VALUE}"

case "${action}" in
  up)
    build=0
    while (($# > 0)); do
      case "$1" in
        --build)
          build=1
          shift
          ;;
        *)
          die "unsupported up option: $1"
          ;;
      esac
    done
    managed_checkout_export_build_commit "${REPO_DIR}"
    compose_cmd="$(resolve_compose_cmd)"
    NO_BUILD=$((1 - build))
    NO_CACHE=0
    NO_RESTART=0
    compose_build "${REPO_DIR}" "${compose_cmd}" "${REPO_DIR}/docker-compose.yml" core "${OMNIDEX_COMMIT}"
    expected_image="$(compose_image_id "${REPO_DIR}" "${compose_cmd}" "${REPO_DIR}/docker-compose.yml" core "${OMNIDEX_COMMIT}")"
    compose_require_image_commit "${expected_image}" "${OMNIDEX_COMMIT}" "${EXPECTED_RUNTIME_USER}"
    compose_restart "${REPO_DIR}" "${compose_cmd}" "${REPO_DIR}/docker-compose.yml" core "${OMNIDEX_COMMIT}"
    compose_require_running_image "${REPO_DIR}" "${compose_cmd}" "${REPO_DIR}/docker-compose.yml" core "${expected_image}" "${OMNIDEX_COMMIT}" "${EXPECTED_RUNTIME_USER}"
    ;;
  down)
    (($# == 0)) || die "down does not accept options"
    compose_cmd="$(resolve_compose_cmd)"
    declare -a cmd=()
    compose_command_array "${compose_cmd}" cmd
    cmd+=(-f "${REPO_DIR}/docker-compose.yml" down --remove-orphans)
    (
      cd "${REPO_DIR}"
      runtime_export_compose_identity
      "${cmd[@]}"
    )
    ;;
esac

if [[ "${action}" == "up" ]]; then
  core_container="$(
    cd "${REPO_DIR}"
    export OMNIDEX_COMMIT
    runtime_export_compose_identity
    compose_docker -p "${COMPOSE_PROJECT}" -f "${REPO_DIR}/docker-compose.yml" ps -q core
  )"
  [[ "${core_container}" =~ ^[0-9a-f]{12,64}$ ]] ||
    die "core did not resolve to one running container after compose health wait"
  context_docker exec "${core_container}" sh -ec 'if [ -n "${HOST_AGENT_URL:-}" ]; then wget -q -O /dev/null "${HOST_AGENT_URL%/}/healthz"; fi; if [ "${LLM_PROVIDER:-}" = "ollama" ] || [ "${EMBEDDING_PROVIDER:-}" = "ollama" ]; then wget -q -O /dev/null "${OLLAMA_BASE_URL%/}/api/tags"; fi' ||
    die "core cannot reach one or more configured host dependencies"
fi
