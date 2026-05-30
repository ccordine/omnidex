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
HOST_ONLY=0
NO_HOST_RESTART=0

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
  --host-only             Only pull latest source and rebuild installed host binaries
  --no-host-restart       Skip restarting the host bridge systemd user service
  -h, --help              Show this help

What this updater does:
  1) Fetches latest git refs and fast-forward pulls to latest
  2) Rebuilds host binaries, including bin/omni
  3) Restarts the host bridge user service when installed (omni-host-bridge)
  4) Rebuilds the Docker image for the selected service
  5) Restarts the selected service with docker compose
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

absolute_existing_path() {
  local raw="$1"
  (
    cd "$raw"
    pwd -P
  )
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

needs_compose_work() {
  if ((HOST_ONLY)); then
    return 1
  fi
  if ((NO_BUILD && NO_RESTART)); then
    return 1
  fi
  return 0
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
    rm -f bin/agent-core bin/agent-cli bin/omni
    go build -o bin/agent-core ./cmd/core
    go build -o bin/agent-cli ./cmd/cli
    go build -o bin/omni ./cmd/omni
    ln -sfn agent-cli bin/acli
  )
}

host_bridge_unit_file() {
  printf '%s\n' "${HOME}/.config/systemd/user/omni-host-bridge.service"
}

host_bridge_exec_start() {
  local unit="$1"
  sed -n 's/^ExecStart=//p' "${unit}" | head -n 1
}

host_bridge_omni_from_exec_start() {
  local exec_start="$1"
  case "${exec_start}" in
    *" host serve")
      printf '%s\n' "${exec_start% host serve}"
      ;;
    *)
      printf '%s\n' ""
      ;;
  esac
}

refresh_host_bridge_binary_for_unit() {
  local repo_dir="$1"
  local unit="$2"
  local built_omni="${repo_dir}/bin/omni"
  local exec_start service_omni service_dir tmp

  exec_start="$(host_bridge_exec_start "${unit}")"
  service_omni="$(host_bridge_omni_from_exec_start "${exec_start}")"
  if [[ -z "${service_omni}" ]]; then
    warn "could not parse host bridge ExecStart: ${exec_start}"
    return 0
  fi
  if [[ "${service_omni}" == "${built_omni}" ]]; then
    return 0
  fi

  warn "host bridge unit uses a different binary: ${exec_start}"
  case "${service_omni}" in
    "${HOME}"/*)
      ;;
    *)
      warn "not refreshing ${service_omni}; it is outside ${HOME}"
      warn "reinstall the bridge with: ${built_omni} host service install --omni ${built_omni}"
      return 0
      ;;
  esac

  service_dir="$(dirname "${service_omni}")"
  mkdir -p "${service_dir}"
  log "refreshing host bridge binary at ${service_omni}"

  tmp="${service_omni}.new.$$"
  install -m 0755 "${built_omni}" "${tmp}"
  mv -f "${tmp}" "${service_omni}"

  if [[ -x "${repo_dir}/bin/agent-core" ]]; then
    tmp="${service_dir}/agent-core.new.$$"
    install -m 0755 "${repo_dir}/bin/agent-core" "${tmp}"
    mv -f "${tmp}" "${service_dir}/agent-core"
  fi
  if [[ -x "${repo_dir}/bin/agent-cli" ]]; then
    tmp="${service_dir}/agent-cli.new.$$"
    install -m 0755 "${repo_dir}/bin/agent-cli" "${tmp}"
    mv -f "${tmp}" "${service_dir}/agent-cli"
    ln -sfn agent-cli "${service_dir}/acli"
  fi
}

restart_host_bridge() {
  local repo_dir="$1"
  local omni="${repo_dir}/bin/omni"
  local unit

  if ((NO_HOST_RESTART)); then
    log "skipping host bridge restart (--no-host-restart)"
    return 0
  fi

  if [[ ! -x "${omni}" ]]; then
    warn "bin/omni not found; skipping host bridge restart"
    return 0
  fi

  unit="$(host_bridge_unit_file)"
  if [[ ! -f "${unit}" ]]; then
    log "host bridge service not installed; skipping restart (run: ${omni} host service install)"
    return 0
  fi

  refresh_host_bridge_binary_for_unit "${repo_dir}" "${unit}"

  log "restarting host bridge (omni-host-bridge)"
  if "${omni}" host service restart; then
    log "host bridge restarted"
    return 0
  fi

  warn "host bridge restart failed; check: ${omni} host service status"
  return 0
}

refresh_installed_payload_permissions() {
  local repo_dir="$1"

  for path in \
    "${repo_dir}/agent_aliases.sh" \
    "${repo_dir}/install.sh" \
    "${repo_dir}/update.sh" \
    "${repo_dir}/uninstall.sh" \
    "${repo_dir}/scripts/build-release.sh" \
    "${repo_dir}/scripts/setup-host-deps.sh" \
    "${repo_dir}/scripts/setup-host-deps.ps1" \
    "${repo_dir}/up.sh" \
    "${repo_dir}/down.sh"; do
    [[ -f "${path}" ]] || continue
    chmod +x "${path}"
  done
}

main() {
  parse_args "$@"

  PREFIX="$(expand_home_path "${PREFIX}")"
  [[ -d "${PREFIX}" ]] || die "prefix path does not exist: ${PREFIX}"
  PREFIX="$(absolute_existing_path "${PREFIX}")"

  local compose_cmd=""
  if needs_compose_work; then
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
    compose_cmd="$(resolve_compose_cmd)"
    log "using compose command: ${compose_cmd}"
  else
    log "skipping docker compose checks (--host-only or --no-build --no-restart)"
  fi

  log "target path: ${PREFIX}"

  git_update_repo "${PREFIX}" "${BRANCH}"
  refresh_installed_payload_permissions "${PREFIX}"
  rebuild_host_binaries "${PREFIX}"
  restart_host_bridge "${PREFIX}"
  if needs_compose_work; then
    compose_build "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" "${SERVICE}"
    compose_restart "${PREFIX}" "${compose_cmd}" "${COMPOSE_FILE}" "${SERVICE}"
  fi

  log "update complete"
}

main "$@"
