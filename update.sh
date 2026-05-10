#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PREFIX="${SCRIPT_DIR}"
BRANCH=""
COMPOSE_FILE=""
SERVICE="core"
NO_PULL=0
NO_BUILD=0
NO_RESTART=0
NO_CACHE=0

usage() {
  cat <<EOF
Usage:
  ./${SCRIPT_NAME} [options]

Options:
  --prefix <path>         Omnidex repo/install path (default: script directory)
  --branch <name>         Git branch to update (default: current branch)
  --compose-file <path>   Compose file to use (default: docker-compose.yml in prefix)
  --service <name>        Compose service to rebuild/restart (default: core)
  --no-cache              Rebuild Docker image without cache
  --no-pull               Skip git fetch/pull
  --no-build              Skip docker compose build
  --no-restart            Skip docker compose up -d
  -h, --help              Show this help

What this updater does:
  1) Fetches latest git refs and fast-forward pulls to latest
  2) Rebuilds the Docker image for the selected service
  3) Restarts the selected service with docker compose
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

has_blocking_git_changes() {
  local repo_dir="$1"
  local line status path
  while IFS= read -r line; do
    [[ -n "${line}" ]] || continue
    status="${line:0:2}"
    path="${line:3}"
    path="${path#\"}"
    path="${path%\"}"
    if [[ "${path}" == *" -> "* ]]; then
      path="${path##* -> }"
      path="${path#\"}"
      path="${path%\"}"
    fi

    # Installer/runtime-generated binaries should not block updates.
    if [[ "${path}" == bin/* ]]; then
      continue
    fi
    # Untracked files are often local artifacts/config and should not block pull.
    if [[ "${status}" == "??" ]]; then
      continue
    fi
    return 0
  done < <(git -C "${repo_dir}" status --porcelain=1 --untracked-files=all)
  return 1
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

resolve_compose_cmd() {
  if command_exists docker && docker compose version >/dev/null 2>&1; then
    printf '%s\n' "docker compose"
    return
  fi
  if command_exists docker-compose; then
    printf '%s\n' "docker-compose"
    return
  fi
  die "docker compose is required but was not found"
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
      --service)
        (($# >= 2)) || die "--service requires a value"
        SERVICE="$2"
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

git_update_repo() {
  local repo_dir="$1"
  local branch="$2"

  if ((NO_PULL)); then
    log "skipping git pull (--no-pull)"
    return 0
  fi

  command_exists git || die "git is required but was not found in PATH"
  [[ -d "${repo_dir}/.git" ]] || die "no .git directory at ${repo_dir}; cannot pull latest"

  if [[ -z "${branch}" ]]; then
    branch="$(git -C "${repo_dir}" rev-parse --abbrev-ref HEAD)"
  fi
  [[ -n "${branch}" ]] || die "unable to resolve git branch"

  if has_blocking_git_changes "${repo_dir}"; then
    die "working tree has tracked changes outside generated bin artifacts; commit/stash before update"
  fi

  log "fetching latest refs"
  git -C "${repo_dir}" fetch --all --prune

  log "checking out branch ${branch}"
  git -C "${repo_dir}" checkout "${branch}"

  log "pulling latest origin/${branch}"
  git -C "${repo_dir}" pull --ff-only origin "${branch}"
}

compose_build() {
  local repo_dir="$1"
  local compose_cmd="$2"
  local compose_file="$3"
  local service="$4"

  if ((NO_BUILD)); then
    log "skipping docker compose build (--no-build)"
    return 0
  fi

  local -a cmd=()
  read -r -a cmd <<<"${compose_cmd}"
  if [[ -n "${compose_file}" ]]; then
    cmd+=(-f "${compose_file}")
  fi
  cmd+=(build --pull)
  if ((NO_CACHE)); then
    cmd+=(--no-cache)
  fi
  cmd+=("${service}")

  log "rebuilding image for service ${service}"
  (
    cd "${repo_dir}"
    "${cmd[@]}"
  )
}

compose_restart() {
  local repo_dir="$1"
  local compose_cmd="$2"
  local compose_file="$3"
  local service="$4"

  if ((NO_RESTART)); then
    log "skipping docker compose up (--no-restart)"
    return 0
  fi

  local -a cmd=()
  read -r -a cmd <<<"${compose_cmd}"
  if [[ -n "${compose_file}" ]]; then
    cmd+=(-f "${compose_file}")
  fi
  cmd+=(up -d --remove-orphans "${service}")

  log "restarting service ${service}"
  (
    cd "${repo_dir}"
    "${cmd[@]}"
  )
}

rebuild_host_binaries() {
  local repo_dir="$1"

  if ! command_exists go; then
    warn "go not found; skipping host binary rebuild (omni/agent-cli may remain stale)"
    return 0
  fi

  log "rebuilding host binaries"
  (
    cd "${repo_dir}"
    mkdir -p bin
    go build -o bin/agent-core ./cmd/core
    go build -o bin/agent-cli ./cmd/cli
    ln -sfn agent-cli bin/omni
    ln -sfn agent-cli bin/acli
  )
}

main() {
  parse_args "$@"

  PREFIX="$(expand_home_path "${PREFIX}")"
  if command_exists realpath; then
    PREFIX="$(realpath -m "${PREFIX}")"
  fi
  [[ -d "${PREFIX}" ]] || die "prefix path does not exist: ${PREFIX}"

  if [[ -z "${COMPOSE_FILE}" ]]; then
    COMPOSE_FILE="${PREFIX}/docker-compose.yml"
  else
    COMPOSE_FILE="$(expand_home_path "${COMPOSE_FILE}")"
    if [[ "${COMPOSE_FILE}" != /* ]]; then
      COMPOSE_FILE="${PREFIX}/${COMPOSE_FILE#./}"
    fi
  fi
  [[ -f "${COMPOSE_FILE}" ]] || die "compose file not found: ${COMPOSE_FILE}"

  [[ -n "${SERVICE}" ]] || die "service cannot be empty"

  local compose_cmd
  compose_cmd="$(resolve_compose_cmd)"
  log "using compose command: ${compose_cmd}"
  log "target path: ${PREFIX}"

  git_update_repo "${PREFIX}" "${BRANCH}"
  rebuild_host_binaries "${PREFIX}"
  compose_build "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" "${SERVICE}"
  compose_restart "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" "${SERVICE}"

  log "update complete"
}

main "$@"
