#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
source "${SCRIPT_DIR}/build-release-lib.sh"

DIST_DIR="${REPO_ROOT}/dist"
VERSION="v0.5.0"
CODENAME="Charmeleon"
TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)
PACKAGES=(
  "omni:./cmd/omni"
  "agent-core:./cmd/core"
  "agent-cli:./cmd/cli"
)

SOURCE_STAGE_ROOT=""
SOURCE_TREE=""
RELEASE_OUTPUT_STAGE=""
RELEASE_COMMIT=""
RELEASE_SOURCE_SHA256=""
RELEASE_MIGRATIONS_SHA256=""
RELEASE_BUILD_DATE=""
EXPECTED_SOURCE_MANIFEST=""

usage() {
  cat <<'EOF'
Usage:
  scripts/build-release.sh [options]

Options:
  --dist <path>       Output directory (default: ./dist)
  --version <value>   Version label used in archive names and binary metadata (default: v0.5.0)
  --codename <value>  Release codename embedded in binary metadata (default: Charmeleon)
  --target <goos/goarch>
                      Build one target. May be repeated. Defaults to linux/darwin/windows amd64+arm64.
  -h, --help          Show this help

Examples:
  scripts/build-release.sh --version v0.5.0 --codename Charmeleon
  scripts/build-release.sh --target darwin/arm64 --target windows/amd64
EOF
}

source_archive_sha256() {
  sha256_file "$1"
}

parse_args() {
  local custom_targets=0
  while (($# > 0)); do
    case "$1" in
      --dist)
        (($# >= 2)) || die "--dist requires a value"
        DIST_DIR="$2"
        shift 2
        ;;
      --version)
        (($# >= 2)) || die "--version requires a value"
        VERSION="$2"
        shift 2
        ;;
      --codename)
        (($# >= 2)) || die "--codename requires a value"
        CODENAME="$2"
        shift 2
        ;;
      --target)
        (($# >= 2)) || die "--target requires a value"
        if ((custom_targets == 0)); then
          TARGETS=()
          custom_targets=1
        fi
        TARGETS+=("$2")
        shift 2
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

archive_target() {
  local target_dir="$1"
  local archive_base="$2"
  local goos="$3"

  (
    cd "$target_dir"
    if [[ "$goos" == "windows" ]]; then
      zip -qr "${RELEASE_OUTPUT_STAGE}/${archive_base}.zip" .
    else
      tar -czf "${RELEASE_OUTPUT_STAGE}/${archive_base}.tar.gz" .
    fi
  )
}

cleanup_source_stage() {
  if [[ -n "$SOURCE_STAGE_ROOT" && "$SOURCE_STAGE_ROOT" != "/" && -d "$SOURCE_STAGE_ROOT" ]]; then
    chmod -R u+w "$SOURCE_STAGE_ROOT" 2>/dev/null || true
    rm -rf -- "$SOURCE_STAGE_ROOT"
  fi
}

prepare_source_stage() {
  RELEASE_COMMIT="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || true)"
  [[ "$RELEASE_COMMIT" =~ ^[0-9a-f]{40}$|^[0-9a-f]{64}$ ]] || die "a full Git commit is required"
  SOURCE_STAGE_ROOT="$(mktemp -d "${DIST_DIR}/.release-stage.XXXXXXXX")"
  [[ -d "$SOURCE_STAGE_ROOT" && ! -L "$SOURCE_STAGE_ROOT" ]] || die "immutable source stage is unavailable"
  SOURCE_TREE="${SOURCE_STAGE_ROOT}/source"
  RELEASE_OUTPUT_STAGE="${SOURCE_STAGE_ROOT}/output"
  mkdir -p "$SOURCE_TREE" "$RELEASE_OUTPUT_STAGE"
  local archive="${SOURCE_STAGE_ROOT}/source.tar"
  git -C "$REPO_ROOT" archive --format=tar --output="$archive" "$RELEASE_COMMIT"
  RELEASE_SOURCE_SHA256="$(source_archive_sha256 "$archive")"
  [[ "$RELEASE_SOURCE_SHA256" =~ ^[0-9a-f]{64}$ ]] || die "tracked source SHA-256 is invalid"
  tar -xf "$archive" -C "$SOURCE_TREE"
  tar -df "$archive" -C "$SOURCE_TREE" >/dev/null || die "extracted source differs from its archive"
  EXPECTED_SOURCE_MANIFEST="${SOURCE_STAGE_ROOT}/source-manifest"
  write_source_manifest "$SOURCE_TREE" "$EXPECTED_SOURCE_MANIFEST"
  chmod 0444 "$archive" "$EXPECTED_SOURCE_MANIFEST"
  chmod -R a-w "$SOURCE_TREE"
  verify_migration_manifest "${SOURCE_TREE}/migrations"
  RELEASE_MIGRATIONS_SHA256="$(sha256_file "${SOURCE_TREE}/migrations/SHA256SUMS")"
  [[ "$RELEASE_MIGRATIONS_SHA256" =~ ^[0-9a-f]{64}$ ]] || die "migration manifest SHA-256 is invalid"
  RELEASE_BUILD_DATE="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
}

verify_source_stage() {
  local archive="${SOURCE_STAGE_ROOT}/source.tar"
  [[ "$(source_archive_sha256 "$archive")" == "$RELEASE_SOURCE_SHA256" ]] ||
    die "immutable source archive changed during release build"
  tar -df "$archive" -C "$SOURCE_TREE" >/dev/null ||
    die "immutable source tree changed during release build"
  local actual="${SOURCE_STAGE_ROOT}/source-manifest.actual"
  write_source_manifest "$SOURCE_TREE" "$actual"
  cmp -s "$EXPECTED_SOURCE_MANIFEST" "$actual" ||
    die "immutable source manifest changed during release build"
  rm -f -- "$actual"
}

prepare_target_source() {
  local target_name="$1" target_source="${SOURCE_STAGE_ROOT}/work-${target_name}"
  verify_source_stage
  mkdir -p "$target_source"
  tar -xf "${SOURCE_STAGE_ROOT}/source.tar" -C "$target_source"
  local actual="${SOURCE_STAGE_ROOT}/${target_name}.manifest"
  write_source_manifest "$target_source" "$actual"
  cmp -s "$EXPECTED_SOURCE_MANIFEST" "$actual" || die "target source extraction changed"
  rm -f -- "$actual"
  chmod -R a-w "$target_source"
  printf '%s\n' "$target_source"
}

build_target() {
  local target="$1"
  local goos="${target%/*}"
  local goarch="${target#*/}"

  [[ -n "$goos" && -n "$goarch" && "$goos" != "$goarch" ]] || die "invalid target: $target"

  local target_name="omnidex-${VERSION}-${goos}-${goarch}"
  local target_source
  target_source="$(prepare_target_source "$target_name")"
  local ldflags
  ldflags="-X github.com/gryph/omnidex/internal/version.Version=${VERSION} -X github.com/gryph/omnidex/internal/version.Codename=${CODENAME} -X github.com/gryph/omnidex/internal/version.Commit=${RELEASE_COMMIT} -X github.com/gryph/omnidex/internal/version.SourceSHA256=${RELEASE_SOURCE_SHA256} -X github.com/gryph/omnidex/internal/version.MigrationsSHA256=${RELEASE_MIGRATIONS_SHA256} -X github.com/gryph/omnidex/internal/version.Date=${RELEASE_BUILD_DATE}"
  local target_dir="${RELEASE_OUTPUT_STAGE}/${target_name}"
  mkdir -p "${target_dir}/bin"

  log "building ${target}"
  local entry name pkg ext
  for entry in "${PACKAGES[@]}"; do
    name="${entry%%:*}"
    pkg="${entry#*:}"
    ext=""
    if [[ "$goos" == "windows" ]]; then
      ext=".exe"
    fi
    (
      cd "$target_source"
      CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$ldflags" -o "${target_dir}/bin/${name}${ext}" "$pkg"
    )
  done

  cp -a "${target_source}/README.md" "${target_dir}/README.md"
  cp -a "${target_source}/LICENSE" "${target_dir}/LICENSE"
  cp -a "${target_source}/migrations" "${target_dir}/migrations"
  verify_migration_manifest "${target_dir}/migrations"
  [[ "$(sha256_file "${target_dir}/migrations/SHA256SUMS")" == "$RELEASE_MIGRATIONS_SHA256" ]] || die "packaged migration manifest changed"
  if [[ -f "${target_source}/CHANGELOG.md" ]]; then
    cp -a "${target_source}/CHANGELOG.md" "${target_dir}/CHANGELOG.md"
  fi
  if [[ -f "${target_source}/agent_aliases.sh" && "$goos" != "windows" ]]; then
    cp -a "${target_source}/agent_aliases.sh" "${target_dir}/agent_aliases.sh"
  fi

  local actual="${SOURCE_STAGE_ROOT}/${target_name}.after.manifest"
  write_source_manifest "$target_source" "$actual"
  cmp -s "$EXPECTED_SOURCE_MANIFEST" "$actual" || die "target source changed during build"
  rm -f -- "$actual"
  verify_source_stage
  archive_target "$target_dir" "$target_name" "$goos"
}

write_release_checksums() {
  (
    cd "$RELEASE_OUTPUT_STAGE"
    local artifacts=()
    local artifact
    for artifact in omnidex-*.tar.gz omnidex-*.zip; do
      [[ -f "$artifact" ]] || continue
      artifacts+=("$artifact")
    done
    ((${#artifacts[@]} > 0)) || die "release staging produced no archives"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "${artifacts[@]}" > SHA256SUMS
    else
      shasum -a 256 "${artifacts[@]}" > SHA256SUMS
    fi
  )
}

publish_staged_release() {
  verify_source_stage
  assert_repository_matches_snapshot "$REPO_ROOT" "$RELEASE_COMMIT" "$DIST_DIR"
  local publication_name="omnidex-${VERSION}"
  [[ "$publication_name" =~ ^omnidex-v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9]+)*$ ]] ||
    die "release publication name is unsafe"
  local publication="${DIST_DIR}/${publication_name}"
  [[ "$(dirname "$publication")" == "$DIST_DIR" && ! -e "$publication" && ! -L "$publication" ]] ||
    die "version-scoped release publication already exists or is unsafe"
  mv "$RELEASE_OUTPUT_STAGE" "$publication"
}

main() {
  parse_args "$@"

  validate_release_inputs
  validate_dist_dir

  command -v go >/dev/null 2>&1 || die "go is required"
  command -v tar >/dev/null 2>&1 || die "tar is required"
  [[ -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=normal)" ]] || die "release builds require a clean tracked and untracked worktree"
  validate_tracked_release_sources "$REPO_ROOT"
  if printf '%s\n' "${TARGETS[@]}" | grep -q '^windows/'; then
    command -v zip >/dev/null 2>&1 || die "zip is required for Windows archives"
  fi
  create_dist_dir

  prepare_source_stage
  trap cleanup_source_stage EXIT

  local target
  for target in "${TARGETS[@]}"; do
    build_target "$target"
  done
  write_release_checksums
  publish_staged_release
  log "release artifacts written to ${DIST_DIR}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
