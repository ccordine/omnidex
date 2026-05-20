#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PREFIX="${HOME}/.omnidex"
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
  --deps-profile <value>   Dependency profile: core|local|all (default: all)
  --with-whisper           Also install whisper CLI via dependency bootstrap
  --skip-deps              Skip host dependency bootstrap step
  --no-sudo                Pass --no-sudo to dependency bootstrap
  -y, --yes                Non-interactive mode (auto-confirm prompts)
  -h, --help               Show this help

What this installer does:
  1) Copies Omnidex runtime files into --prefix
  2) Builds binaries (bin/omni, bin/agent-core, bin/agent-cli, bin/acli)
  3) Installs host dependencies via scripts/setup-host-deps.sh (unless --skip-deps)
  4) Adds a managed shell-init block so aliases are loaded automatically
EOF
}

log() {
  printf '[install] %s\n' "$*"
}

warn() {
  printf '[install][warn] %s\n' "$*" >&2
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

remove_managed_block_file() {
  local file="$1"
  [[ -f "$file" ]] || return 0

  local tmp
  tmp="$(mktemp)"
  awk -v start="$MANAGED_BLOCK_START" -v end="$MANAGED_BLOCK_END" '
    index($0, start) { skipping=1; next }
    index($0, end) { skipping=0; next }
    !skipping { print }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
}

append_managed_block_file() {
  local file="$1"
  remove_managed_block_file "$file"

  cat >>"$file" <<EOF

${MANAGED_BLOCK_START}
export OMNIDEX_DIR="${PREFIX}"
if [ -d "\$OMNIDEX_DIR/bin" ]; then
  case ":\$PATH:" in
    *":\$OMNIDEX_DIR/bin:"*) ;;
    *) export PATH="\$OMNIDEX_DIR/bin:\$PATH" ;;
  esac
fi
if [ -f "\$OMNIDEX_DIR/agent_aliases.sh" ]; then
  . "\$OMNIDEX_DIR/agent_aliases.sh"
fi
${MANAGED_BLOCK_END}
EOF
}

collect_shell_init_files() {
  local -a found=()
  local file
  for file in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile" "$HOME/.zshrc"; do
    if [[ -f "$file" ]]; then
      found+=("$file")
    fi
  done

  if ((${#found[@]} == 0)); then
    local fallback="$HOME/.bashrc"
    if [[ "$(basename "${SHELL:-bash}")" == "zsh" ]]; then
      fallback="$HOME/.zshrc"
    fi
    touch "$fallback"
    found+=("$fallback")
  fi

  printf '%s\n' "${found[@]}"
}

integrate_shell_init() {
  local file
  local count=0
  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    append_managed_block_file "$file"
    log "updated shell init: $file"
    count=$((count + 1))
  done < <(collect_shell_init_files)

  if ((count == 0)); then
    warn "no shell init files were updated"
  fi
}

copy_runtime_payload() {
  local -a payload_items=(
    install.sh
    update.sh
    uninstall.sh
    agent_aliases.sh
    cmd
    internal
    migrations
    scripts
    .git
    go.mod
    go.sum
    README.md
    Makefile
    default.env
    .env.example
    docker-compose.yml
    Dockerfile
    up.sh
    down.sh
  )

  mkdir -p "$PREFIX"
  local item
  for item in "${payload_items[@]}"; do
    if [[ ! -e "${SCRIPT_DIR}/${item}" ]]; then
      warn "missing source payload item: ${item}"
      continue
    fi
    if [[ -d "${SCRIPT_DIR}/${item}" ]]; then
      rm -rf "${PREFIX:?}/${item}"
    fi
    cp -a "${SCRIPT_DIR}/${item}" "${PREFIX}/${item}"
  done

  chmod +x "${PREFIX}/agent_aliases.sh"
  chmod +x "${PREFIX}/install.sh"
  chmod +x "${PREFIX}/update.sh"
  chmod +x "${PREFIX}/uninstall.sh"
  chmod +x "${PREFIX}/scripts/setup-host-deps.sh"
  [[ -f "${PREFIX}/up.sh" ]] && chmod +x "${PREFIX}/up.sh"
  [[ -f "${PREFIX}/down.sh" ]] && chmod +x "${PREFIX}/down.sh"

  if [[ ! -f "${PREFIX}/.env" && -f "${PREFIX}/default.env" ]]; then
    cp -a "${PREFIX}/default.env" "${PREFIX}/.env"
    log "created ${PREFIX}/.env from default.env"
  fi
}

run_dependency_bootstrap() {
  if ((SKIP_DEPS)); then
    log "skipping dependency bootstrap (--skip-deps)"
    return 0
  fi

  local -a cmd=("${PREFIX}/scripts/setup-host-deps.sh" "--profile" "${DEPS_PROFILE}")
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

build_binaries() {
  if ! command_exists go; then
    die "go is required to build Omnidex binaries (install Go or rerun without --skip-deps)"
  fi
  mkdir -p "${PREFIX}/bin"
  (
    cd "${PREFIX}"
    go build -o bin/agent-core ./cmd/core
    go build -o bin/agent-cli ./cmd/cli
    go build -o bin/omni ./cmd/omni
  )
  ln -sfn agent-cli "${PREFIX}/bin/acli"
  log "built binaries in ${PREFIX}/bin"
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
  mkdir -p "$PREFIX"
  if command_exists realpath; then
    PREFIX="$(realpath -m "$PREFIX")"
  fi
  case "$PREFIX" in
    ""|"/"|"$HOME")
      die "refusing to install into unsafe prefix: ${PREFIX}"
      ;;
  esac

  if [[ -f "${PREFIX}/agent_aliases.sh" || -d "${PREFIX}/cmd" ]]; then
    if ! confirm "Update existing Omnidex install at ${PREFIX}?"; then
      die "installation canceled"
    fi
  fi

  log "install target: ${PREFIX}"
  copy_runtime_payload
  run_dependency_bootstrap
  build_binaries
  integrate_shell_init

  cat <<EOF
[install] completed
[install] omni aliases now auto-load from: ${PREFIX}/agent_aliases.sh
[install] open a new shell (or run: source ~/.bashrc) to use omni immediately
EOF
}

main "$@"
