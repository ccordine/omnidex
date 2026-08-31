#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"

PREFIX="${HOME}/.omnidex"
ASSUME_YES=0
PURGE_CONFIG=0

MANAGED_BLOCK_START="# >>> omnidex install >>>"
MANAGED_BLOCK_END="# <<< omnidex install <<<"

usage() {
  cat <<EOF
Usage:
  ./${SCRIPT_NAME} [options]

Options:
  --prefix <path>      Install path to remove (default: ${HOME}/.omnidex)
  --purge-config       Also remove ${HOME}/.config/omni permissions state
  -y, --yes            Non-interactive mode (auto-confirm prompts)
  -h, --help           Show this help

What this uninstaller does:
  1) Removes Omnidex shell-init blocks from bash/zsh/profile files
  2) Deletes the install directory at --prefix
  3) Optionally removes global Omnidex config under ~/.config/omni
EOF
}

log() {
  printf '[uninstall] %s\n' "$*"
}

warn() {
  printf '[uninstall][warn] %s\n' "$*" >&2
}

die() {
  printf '[uninstall][error] %s\n' "$*" >&2
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
  if ! grep -Fq "$MANAGED_BLOCK_START" "$file"; then
    return 0
  fi

  local tmp
  tmp="$(mktemp)"
  awk -v start="$MANAGED_BLOCK_START" -v end="$MANAGED_BLOCK_END" '
    index($0, start) { skipping=1; next }
    index($0, end) { skipping=0; next }
    !skipping { print }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
  log "removed shell init block from: $file"
}

remove_shell_integration() {
  local file
  for file in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile" "$HOME/.zshrc"; do
    remove_managed_block_file "$file"
  done
}

remove_install_tree() {
  if [[ ! -e "$PREFIX" ]]; then
    log "install path not found: $PREFIX"
    return 0
  fi
  if ! confirm "Delete install directory ${PREFIX}?"; then
    warn "skipped removing install directory"
    return 0
  fi
  rm -rf "$PREFIX"
  log "removed install directory: $PREFIX"
}

purge_config_if_requested() {
  if ((PURGE_CONFIG == 0)); then
    return 0
  fi
  local config_dir="${HOME}/.config/omni"
  if [[ ! -e "$config_dir" ]]; then
    log "config path not found: $config_dir"
    return 0
  fi
  if ! confirm "Delete Omnidex config directory ${config_dir}?"; then
    warn "skipped removing config directory"
    return 0
  fi
  rm -rf "$config_dir"
  log "removed config directory: $config_dir"
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --prefix)
        (($# >= 2)) || die "--prefix requires a value"
        PREFIX="$2"
        shift 2
        ;;
      --purge-config)
        PURGE_CONFIG=1
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
}

main() {
  parse_args "$@"

  PREFIX="$(expand_home_path "$PREFIX")"
  if command_exists realpath; then
    PREFIX="$(realpath -m "$PREFIX")"
  fi
  case "$PREFIX" in
    ""|"/"|"$HOME")
      die "refusing to remove unsafe prefix: ${PREFIX}"
      ;;
  esac

  remove_shell_integration
  remove_install_tree
  purge_config_if_requested

  cat <<EOF
[uninstall] completed
[uninstall] open a new shell to drop omni aliases from active sessions
EOF
}

main "$@"
