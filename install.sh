#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/scripts/managed-checkout-lib.sh"
source "${SCRIPT_DIR}/scripts/install-shell-lib.sh"

PREFIX="${HOME}/.omnidex"
ENV_FILE=""
DEPS_PROFILE="all"
WITH_WHISPER=0
SKIP_DEPS=0
ASSUME_YES=0
NO_SUDO=0

MANAGED_BLOCK_START="# >>> omnidex install >>>"
MANAGED_BLOCK_END="# <<< omnidex install <<<"

usage() {
  cat <<EOF
Usage:
  ./${SCRIPT_NAME} [options]

Options:
  --prefix <path>          Install path (default: ${HOME}/.omnidex)
  --env-file <path>        Required deployment environment for a fresh install
  --deps-profile <value>   Dependency profile: core|local|all (default: all)
  --with-whisper           Also install whisper CLI via dependency bootstrap
  --skip-deps              Skip host dependency bootstrap step
  --no-sudo                Pass --no-sudo to dependency bootstrap
  -y, --yes                Non-interactive mode (auto-confirm prompts)
  -h, --help               Show this help

What this installer does:
  1) Stages a complete, updateable checkout of the exact source HEAD
  2) Reproducibly builds the embedded GUI and all host binaries
  3) Installs host dependencies via scripts/setup-host-deps.sh (unless --skip-deps)
  4) Adds a managed shell-init block so aliases are loaded automatically
EOF
}

log() {
  printf '[install] %s\n' "$*"
}

die() {
  printf '[install][error] %s\n' "$*" >&2
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

absolute_target_path() {
  local raw="$1"
  if [[ "${raw}" != /* ]]; then
    raw="${PWD}/${raw#./}"
  fi
  local parent name
  parent="$(dirname "${raw}")"
  name="$(basename "${raw}")"
  [[ "${name}" != "." && "${name}" != ".." ]] || die "install target name is unsafe"
  mkdir -p "${parent}"
  parent="$(cd "${parent}" && pwd -P)"
  printf '%s/%s\n' "${parent}" "${name}"
}

confirm() {
  local prompt="$1"
  if ((ASSUME_YES)); then
    return 0
  fi
  printf '%s [y/N] ' "$prompt"
  read -r reply
  case "${reply,,}" in
    y|yes)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

run_dependency_bootstrap() {
  if ((SKIP_DEPS)); then
    log "skipping dependency bootstrap (--skip-deps)"
    return 0
  fi

  local -a cmd=("${SCRIPT_DIR}/scripts/setup-host-deps.sh" "--profile" "${DEPS_PROFILE}")
  if ((WITH_WHISPER)); then
    cmd+=("--with-whisper")
  fi
  if ((ASSUME_YES)); then
    cmd+=("--yes")
  fi
  if ((NO_SUDO)); then
    cmd+=("--no-sudo")
  fi

  log "running dependency bootstrap (${DEPS_PROFILE})"
  "${cmd[@]}"
}

build_staged_checkout() {
  local repository="$1"
  if ! command_exists go; then
    die "go is required to build Omnidex binaries (install Go or rerun without --skip-deps)"
  fi
  "${repository}/scripts/build-ui.sh"
  mkdir -p "${repository}/bin"
  (
    cd "${repository}"
    build_dir="$(mktemp -d "${repository}/bin/.omnidex-build.XXXXXX")"
    trap 'rm -f "${build_dir}/agent-core" "${build_dir}/agent-cli" "${build_dir}/omni"; rmdir "${build_dir}" 2>/dev/null || true' EXIT
    go build -o "${build_dir}/agent-core" ./cmd/core
    go build -o "${build_dir}/agent-cli" ./cmd/cli
    go build -o "${build_dir}/omni" ./cmd/omni
    mv -f "${build_dir}/agent-core" bin/agent-core
    mv -f "${build_dir}/agent-cli" bin/agent-cli
    mv -f "${build_dir}/omni" bin/omni
  )
  ln -sfn agent-cli "${repository}/bin/acli"
  log "built staged GUI and binaries"
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --prefix)
        (($# >= 2)) || die "--prefix requires a value"
        PREFIX="$2"
        shift 2
        ;;
      --deps-profile)
        (($# >= 2)) || die "--deps-profile requires a value"
        DEPS_PROFILE="$2"
        shift 2
        ;;
      --env-file)
        (($# >= 2)) || die "--env-file requires a value"
        ENV_FILE="$2"
        shift 2
        ;;
      --with-whisper)
        WITH_WHISPER=1
        shift
        ;;
      --skip-deps)
        SKIP_DEPS=1
        shift
        ;;
      --no-sudo)
        NO_SUDO=1
        shift
        ;;
      -y|--yes)
        ASSUME_YES=1
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

  case "${DEPS_PROFILE}" in
    core|local|all)
      ;;
    *)
      die "invalid --deps-profile value: ${DEPS_PROFILE} (use core|local|all)"
      ;;
  esac
}

main() {
  parse_args "$@"

  PREFIX="$(expand_home_path "${PREFIX}")"
  PREFIX="$(absolute_target_path "${PREFIX}")"
  if [[ -n "${ENV_FILE}" ]]; then
    ENV_FILE="$(expand_home_path "${ENV_FILE}")"
    if [[ "${ENV_FILE}" != /* ]]; then
      ENV_FILE="${PWD}/${ENV_FILE#./}"
    fi
  fi
  case "$PREFIX" in
    ""|"/"|"$HOME")
      die "refusing to install into unsafe prefix: ${PREFIX}"
      ;;
  esac
  [[ ! -L "${PREFIX}" ]] || die "refusing to replace a symlink install target: ${PREFIX}"
  case "${PREFIX}/" in
    "${SCRIPT_DIR}/"|"${SCRIPT_DIR}/"*) die "install target must not overlap the source checkout" ;;
  esac
  case "${SCRIPT_DIR}/" in
    "${PREFIX}/"*) die "install target must not contain the source checkout" ;;
  esac

  managed_checkout_require_source "${SCRIPT_DIR}"
  local source_branch source_origin stage=""
  source_branch="$(managed_checkout_branch "${SCRIPT_DIR}" "")"
  source_origin="$(managed_checkout_origin "${SCRIPT_DIR}")"

  if [[ -e "${PREFIX}" ]]; then
    [[ -d "${PREFIX}" ]] || die "install target exists and is not a directory: ${PREFIX}"
    if [[ -n "$(find "${PREFIX}" -mindepth 1 -maxdepth 1 -print -quit)" && ! -d "${PREFIX}/.git" ]]; then
      die "existing non-empty target is not a managed Omnidex checkout: ${PREFIX}"
    fi
    if ! confirm "Update existing Omnidex install at ${PREFIX}?"; then
      die "installation canceled"
    fi
  fi

  log "install target: ${PREFIX}"
  run_dependency_bootstrap
  stage="$(managed_checkout_new_stage "${PREFIX}" "install")"
  trap '[[ -z "${stage:-}" ]] || rm -rf -- "${stage}"' EXIT
  managed_checkout_clone_exact "${SCRIPT_DIR}" "${stage}" "${source_branch}" "${source_origin}"
  managed_checkout_stage_env "${PREFIX}" "${stage}" "${ENV_FILE}"
  build_staged_checkout "${stage}"
  managed_checkout_validate_env "${stage}"
  managed_checkout_publish "${stage}" "${PREFIX}"
  stage=""
  trap - EXIT
  integrate_shell_init

  cat <<EOF
[install] completed
[install] omni aliases now auto-load from: ${PREFIX}/agent_aliases.sh
[install] open a new shell (or run: source ~/.bashrc) to use omni immediately
EOF
}

main "$@"
