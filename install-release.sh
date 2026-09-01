#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/scripts/managed-release-install-lib.sh"
source "${SCRIPT_DIR}/scripts/install-shell-lib.sh"

PREFIX="${HOME}/.omnidex"
ENV_FILE=""
ASSUME_YES=0
MANAGED_BLOCK_START="# >>> omnidex install >>>"
MANAGED_BLOCK_END="# <<< omnidex install <<<"

log() { printf '[install-release] %s\n' "$*"; }
die() { printf '[install-release][error] %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: ./install-release.sh [--prefix path] [--env-file path] [--yes]

Installs this Unix native release archive atomically. A fresh install requires an
explicit --env-file. An existing managed install preserves its regular .env
byte-for-byte and rejects --env-file replacement. The installed core preserves no
database upgrade state: every startup rebuilds DATABASE_SCHEMA from database/setup.sql.
EOF
}

while (($# > 0)); do
  case "$1" in
    --prefix)
      (($# >= 2)) || die "--prefix requires a value"
      PREFIX="$2"
      shift 2
      ;;
    --env-file)
      (($# >= 2)) || die "--env-file requires a value"
      ENV_FILE="$2"
      shift 2
      ;;
    -y|--yes)
      ASSUME_YES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *) die "unknown option: $1" ;;
  esac
done

case "${PREFIX}" in
  "~") PREFIX="${HOME}" ;;
  "~/"*) PREFIX="${HOME}/${PREFIX#~/}" ;;
esac
if [[ "${PREFIX}" != /* ]]; then
  PREFIX="${PWD}/${PREFIX#./}"
fi
case "${PREFIX}" in
  ""|"/"|"${HOME}") die "refusing unsafe install prefix: ${PREFIX}" ;;
esac
if [[ -n "${ENV_FILE}" && "${ENV_FILE}" != /* ]]; then
  ENV_FILE="${PWD}/${ENV_FILE#./}"
fi

source_root="$(managed_release_root "${SCRIPT_DIR}")"
if [[ "${PREFIX}" == "${source_root}" ]]; then
  die "install prefix must differ from the extracted release archive"
fi
if [[ -e "${PREFIX}" && ${ASSUME_YES} -eq 0 ]]; then
  printf 'Replace managed install at %s? [y/N] ' "${PREFIX}"
  read -r answer
  [[ "${answer,,}" == "y" || "${answer,,}" == "yes" ]] || die "installation canceled"
fi

trap managed_release_cleanup_stage EXIT
managed_release_publish "${source_root}" "${PREFIX}" "${ENV_FILE}"
PREFIX="${MANAGED_RELEASE_TARGET}"
integrate_shell_init
log "installed ${PREFIX}"
