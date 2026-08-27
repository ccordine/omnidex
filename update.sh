#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/scripts/managed-checkout-lib.sh"
source "${SCRIPT_DIR}/scripts/update-runtime-lib.sh"

PREFIX="${SCRIPT_DIR}"
BRANCH=""
COMPOSE_FILE=""
NO_PULL=0
NO_BUILD=0
NO_RESTART=0
NO_CACHE=0
HOST_ONLY=0
NO_HOST_RESTART=0
DOCKER_CONTEXT_NAME=""
COMPOSE_PROJECT=""

usage() {
  cat <<EOF
Usage:
  ./${SCRIPT_NAME} [options]

Options:
  --prefix <path>         Omnidex repo/install path (default: script directory)
  --branch <name>         Git branch to update (default: current branch)
  --compose-file <path>   Compose file to use (default: docker-compose.yml in prefix)
  --no-cache              Rebuild Docker image without cache
  --no-pull               Skip git fetch/pull
  --no-build              Skip docker compose build
  --no-restart            Skip docker compose up -d
  --host-only             Only pull latest source and rebuild installed host binaries
  --no-host-restart       Skip restarting the host bridge systemd user service
  -h, --help              Show this help

What this updater does:
  1) Stages a complete checkout and fast-forwards it to latest
  2) Reproducibly builds the embedded GUI and all host binaries
  3) Restarts the host bridge user service when installed (omni-host-bridge)
  4) Rebuilds the Docker image for the core service
  5) Restarts the core service with docker compose
EOF
}

log() {
  printf '[update] %s\n' "$*"
}

warn() {
  printf '[update][warn] %s\n' "$*" >&2
}

die() {
  printf '[update][error] %s\n' "$*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

expand_home_path() {
  local raw="$1"
  case "$raw" in
    "~")
      printf '%s\n' "$HOME"
      ;;
    "~/"*)
      printf '%s\n' "${HOME}/${raw#~/}"
      ;;
    *)
      printf '%s\n' "$raw"
      ;;
  esac
}

absolute_existing_path() {
  local raw="$1"
  (
    cd "$raw"
    pwd -P
  )
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --prefix)
        (($# >= 2)) || die "--prefix requires a value"
        PREFIX="$2"
        shift 2
        ;;
      --branch)
        (($# >= 2)) || die "--branch requires a value"
        BRANCH="$2"
        shift 2
        ;;
      --compose-file)
        (($# >= 2)) || die "--compose-file requires a value"
        COMPOSE_FILE="$2"
        shift 2
        ;;
      --no-cache)
        NO_CACHE=1
        shift
        ;;
      --no-pull)
        NO_PULL=1
        shift
        ;;
      --no-build)
        NO_BUILD=1
        shift
        ;;
      --no-restart)
        NO_RESTART=1
        shift
        ;;
      --host-only)
        HOST_ONLY=1
        NO_BUILD=1
        NO_RESTART=1
        shift
        ;;
      --no-host-restart)
        NO_HOST_RESTART=1
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "unknown option: $1 (use --help)"
        ;;
    esac
  done
}

build_staged_checkout() {
  local repo_dir="$1"

  if ! command_exists go; then
    die "go is required to build Omnidex binaries"
  fi

  managed_checkout_export_build_commit "${repo_dir}"
  log "building staged GUI and host binaries"
  "${repo_dir}/scripts/build-ui.sh"
  (
    cd "${repo_dir}"
    ldflags="-X github.com/gryph/omnidex/internal/version.Commit=${OMNIDEX_COMMIT}"
    mkdir -p bin
    build_dir="$(mktemp -d "${repo_dir}/bin/.omnidex-build.XXXXXX")"
    trap 'rm -f "${build_dir}/agent-core" "${build_dir}/agent-cli" "${build_dir}/omni"; rmdir "${build_dir}" 2>/dev/null || true' EXIT
    go build -trimpath -ldflags "${ldflags}" -o "${build_dir}/agent-core" ./cmd/core
    go build -trimpath -ldflags "${ldflags}" -o "${build_dir}/agent-cli" ./cmd/cli
    go build -trimpath -ldflags "${ldflags}" -o "${build_dir}/omni" ./cmd/omni
    managed_checkout_verify_binary_commit "${build_dir}/agent-core" "${OMNIDEX_COMMIT}" core
    managed_checkout_verify_binary_commit "${build_dir}/agent-cli" "${OMNIDEX_COMMIT}" json
    managed_checkout_verify_binary_commit "${build_dir}/omni" "${OMNIDEX_COMMIT}" json
    mv -f "${build_dir}/agent-core" bin/agent-core
    mv -f "${build_dir}/agent-cli" bin/agent-cli
    mv -f "${build_dir}/omni" bin/omni
    ln -sfn agent-cli bin/acli
  )
}

main() {
  parse_args "$@"

  PREFIX="$(expand_home_path "${PREFIX}")"
  [[ ! -L "${PREFIX}" ]] || die "update target must not be a symlink: ${PREFIX}"
  [[ -d "${PREFIX}" ]] || die "prefix path does not exist: ${PREFIX}"
  PREFIX="$(absolute_existing_path "${PREFIX}")"
  managed_checkout_require_replaceable_target "${PREFIX}"
  local update_branch update_origin stage=""
  update_branch="$(managed_checkout_branch "${PREFIX}" "${BRANCH}")"
  update_origin="$(managed_checkout_origin "${PREFIX}")"

  local compose_cmd="" expected_image="" expected_runtime_user=""
  if needs_compose_work; then
    managed_checkout_require_env_key "${PREFIX}/.env" "DOCKER_CONTEXT"
    managed_checkout_require_env_key "${PREFIX}/.env" "COMPOSE_PROJECT_NAME"
    managed_checkout_require_env_key "${PREFIX}/.env" "HOST_UID"
    managed_checkout_require_env_key "${PREFIX}/.env" "HOST_GID"
    DOCKER_CONTEXT_NAME="$(managed_checkout_env_value "${PREFIX}/.env" "DOCKER_CONTEXT")"
    COMPOSE_PROJECT="$(managed_checkout_env_value "${PREFIX}/.env" "COMPOSE_PROJECT_NAME")"
    validate_compose_identity "DOCKER_CONTEXT" "${DOCKER_CONTEXT_NAME}"
    validate_compose_identity "COMPOSE_PROJECT_NAME" "${COMPOSE_PROJECT}"
    [[ -n "${DOCKER_CONTEXT_NAME}" ]] || die "DOCKER_CONTEXT must be explicit and non-empty"
    [[ -n "${COMPOSE_PROJECT}" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"
    HOST_UID_VALUE="$(managed_checkout_env_value "${PREFIX}/.env" "HOST_UID")"
    HOST_GID_VALUE="$(managed_checkout_env_value "${PREFIX}/.env" "HOST_GID")"
    expected_runtime_user="$(runtime_user_identity "${HOST_UID_VALUE}" "${HOST_GID_VALUE}")"
    export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT}"
    export HOST_UID="${HOST_UID_VALUE}"
    export HOST_GID="${HOST_GID_VALUE}"
    if [[ -z "${COMPOSE_FILE}" ]]; then
      COMPOSE_FILE="${PREFIX}/docker-compose.yml"
    else
      COMPOSE_FILE="$(expand_home_path "${COMPOSE_FILE}")"
      if [[ "${COMPOSE_FILE}" != /* ]]; then
        COMPOSE_FILE="${PREFIX}/${COMPOSE_FILE#./}"
      fi
    fi
    [[ -f "${COMPOSE_FILE}" ]] || die "compose file not found: ${COMPOSE_FILE}"
    compose_cmd="$(resolve_compose_cmd)"
    log "using compose command: ${compose_cmd}"
  else
    log "skipping docker compose checks (--host-only or --no-build --no-restart)"
  fi

  log "target path: ${PREFIX}"
  stage="$(managed_checkout_new_stage "${PREFIX}" "update")"
  trap '[[ -z "${stage:-}" ]] || rm -rf -- "${stage}"' EXIT
  managed_checkout_clone_exact "${PREFIX}" "${stage}" "${update_branch}" "${update_origin}"
  if ((NO_PULL)); then
    log "skipping remote fast-forward (--no-pull)"
  else
    log "fast-forwarding staged checkout from origin/${update_branch}"
    managed_checkout_fast_forward "${stage}" "${update_branch}"
  fi
  managed_checkout_stage_env "${PREFIX}" "${stage}" ""
  build_staged_checkout "${stage}"
  managed_checkout_validate_env "${stage}"
  managed_checkout_publish "${stage}" "${PREFIX}"
  stage=""
  trap - EXIT
  restart_host_bridge "${PREFIX}"
  if needs_compose_work; then
    compose_build "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" core "${OMNIDEX_COMMIT}"
    expected_image="$(compose_image_id "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" core "${OMNIDEX_COMMIT}")"
    compose_require_image_commit "${expected_image}" "${OMNIDEX_COMMIT}" "${expected_runtime_user}"
    compose_restart "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" core "${OMNIDEX_COMMIT}"
    if ((NO_RESTART == 0)); then
      compose_require_running_image "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" core "${expected_image}" "${OMNIDEX_COMMIT}" "${expected_runtime_user}"
    fi
  fi

  log "update complete"
}

main "$@"
