#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

die() {
  printf '[compose][error] %s\n' "$*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

source "${SCRIPT_DIR}/managed-checkout-lib.sh"
source "${SCRIPT_DIR}/update-runtime-lib.sh"

usage() {
  cat <<EOF
Usage: ./scripts/compose-deployment.sh <up|down> [--build]

Uses the exact DOCKER_CONTEXT and COMPOSE_PROJECT_NAME from .env. It never
removes PostgreSQL or Redis volumes.
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
managed_checkout_require_env_key "${ENV_FILE}" "DOCKER_CONTEXT"
managed_checkout_require_env_key "${ENV_FILE}" "COMPOSE_PROJECT_NAME"
DOCKER_CONTEXT_NAME="$(managed_checkout_env_value "${ENV_FILE}" "DOCKER_CONTEXT")"
COMPOSE_PROJECT="$(managed_checkout_env_value "${ENV_FILE}" "COMPOSE_PROJECT_NAME")"
validate_compose_identity "DOCKER_CONTEXT" "${DOCKER_CONTEXT_NAME}"
validate_compose_identity "COMPOSE_PROJECT_NAME" "${COMPOSE_PROJECT}"
[[ -n "${COMPOSE_PROJECT}" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"

compose_cmd="$(resolve_compose_cmd)"
declare -a cmd=()
compose_command_array "${compose_cmd}" cmd
cmd+=(-f "${REPO_DIR}/docker-compose.yml")

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
    cmd+=(up -d --remove-orphans)
    ((build == 0)) || cmd+=(--build)
    ;;
  down)
    (($# == 0)) || die "down does not accept options"
    cmd+=(down --remove-orphans)
    ;;
esac

cd "${REPO_DIR}"
"${cmd[@]}"
