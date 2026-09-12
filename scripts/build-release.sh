#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
source "${SCRIPT_DIR}/build-release-lib.sh"

DIST_DIR="${REPO_ROOT}/dist"
VERSION="v0.5.0"
CODENAME="Charmeleon"
TARGETS=()
PACKAGES=("omnidex:./cmd/omnidex" "omni:./cmd/omni")
RUNTIME_FILES=(
  ".env.example"
  "install-release.sh"
  "scripts/install-shell-lib.sh"
  "scripts/managed-release-install-lib.sh"
)
RELEASE_STAGE=""
RELEASE_OUTPUT_STAGE=""
RELEASE_BUILD_DATE=""

usage() {
  printf '%s\n' \
    'Usage: scripts/build-release.sh [--dist PATH] [--version VERSION] [--codename NAME] [--target GOOS/GOARCH]' \
    '' \
    'Builds the current workspace into a native release containing omnidex and omni.' \
    'The target defaults to the host. CGO targets require a matching native host.' \
    'An existing version directory is never overwritten.'
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --dist|--version|--codename|--target)
        (($# >= 2)) || die "$1 requires a value"
        case "$1" in
          --dist) DIST_DIR="$2" ;;
          --version) VERSION="$2" ;;
          --codename) CODENAME="$2" ;;
          --target) TARGETS+=("$2") ;;
        esac
        shift 2
        ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown option: $1 (use --help)" ;;
    esac
  done
}

cleanup_release_stage() {
  if [[ -n "${RELEASE_STAGE}" ]]; then
    [[ "${RELEASE_STAGE}" == "${DIST_DIR}/.omnidex-release."* ]] ||
      die "release cleanup target is not the allocated staging directory"
    rm -rf -- "${RELEASE_STAGE}"
  fi
}

copy_release_layout() {
  local target_dir="$1" goos="$2" relative
  cp -p "${REPO_ROOT}/README.md" "${REPO_ROOT}/LICENSE" "${target_dir}/"
  if [[ -f "${REPO_ROOT}/CHANGELOG.md" ]]; then
    cp -p "${REPO_ROOT}/CHANGELOG.md" "${target_dir}/"
  fi
  if [[ "${goos}" != "windows" ]]; then
    for relative in "${RUNTIME_FILES[@]}"; do
      [[ -f "${REPO_ROOT}/${relative}" ]] || die "release source is missing ${relative}"
      mkdir -p "${target_dir}/$(dirname "${relative}")"
      cp -p "${REPO_ROOT}/${relative}" "${target_dir}/${relative}"
    done
  fi
}

build_target() {
  local target="$1" goos="${1%/*}" goarch="${1#*/}"
  local target_name="omnidex-${VERSION}-${goos}-${goarch}"
  local target_dir="${RELEASE_OUTPUT_STAGE}/${target_name}"
  local ldflags entry name package suffix=""
  [[ "${goos}" != "windows" ]] || suffix=".exe"
  ldflags="-X github.com/gryph/omnidex/internal/version.Version=${VERSION} -X github.com/gryph/omnidex/internal/version.Codename=${CODENAME} -X github.com/gryph/omnidex/internal/version.Date=${RELEASE_BUILD_DATE}"
  mkdir -p "${target_dir}/bin"
  log "building ${target}"
  for entry in "${PACKAGES[@]}"; do
    name="${entry%%:*}"
    package="${entry#*:}"
    CGO_ENABLED=1 "${SCRIPT_DIR}/build-core.sh" \
      --package "${package}" --goos "${goos}" --goarch "${goarch}" \
      --ldflags "${ldflags}" --output "${target_dir}/bin/${name}${suffix}"
    [[ -x "${target_dir}/bin/${name}${suffix}" ]] || die "build did not produce ${name}${suffix}"
  done
  copy_release_layout "${target_dir}" "${goos}"
  (
    cd "${target_dir}"
    if [[ "${goos}" == "windows" ]]; then
      zip -qr "${RELEASE_OUTPUT_STAGE}/${target_name}.zip" .
    else
      tar -czf "${RELEASE_OUTPUT_STAGE}/${target_name}.tar.gz" .
    fi
  )
}

main() {
  parse_args "$@"
  command -v go >/dev/null 2>&1 || die "go is required"
  if (("${#TARGETS[@]}" == 0)); then
    TARGETS=("$(go env GOOS)/$(go env GOARCH)")
  fi
  validate_release_inputs
  validate_release_cgo_targets "${TARGETS[@]}"
  validate_dist_dir
  command -v tar >/dev/null 2>&1 || die "tar is required"
  local target publication="${DIST_DIR}/omnidex-${VERSION}"
  for target in "${TARGETS[@]}"; do
    if [[ "${target}" == windows/* ]]; then
      command -v zip >/dev/null 2>&1 || die "zip is required for Windows archives"
    fi
  done
  [[ ! -e "${publication}" && ! -L "${publication}" ]] || die "release already exists: ${publication}"
  mkdir -p "${DIST_DIR}"
  RELEASE_STAGE="$(mktemp -d "${DIST_DIR}/.omnidex-release.XXXXXX")"
  trap cleanup_release_stage EXIT
  RELEASE_OUTPUT_STAGE="${RELEASE_STAGE}/output"
  mkdir "${RELEASE_OUTPUT_STAGE}"
  RELEASE_BUILD_DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  for target in "${TARGETS[@]}"; do
    build_target "${target}"
  done
  [[ ! -e "${publication}" && ! -L "${publication}" ]] || die "release appeared during build: ${publication}"
  mv "${RELEASE_OUTPUT_STAGE}" "${publication}"
  log "release artifacts written to ${publication}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
