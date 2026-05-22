#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

DIST_DIR="${REPO_ROOT}/dist"
VERSION="v0.2.0"
CODENAME="Ivysaur"
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

usage() {
  cat <<'EOF'
Usage:
  scripts/build-release.sh [options]

Options:
  --dist <path>       Output directory (default: ./dist)
  --version <value>   Version label used in archive names and binary metadata (default: v0.2.0)
  --codename <value>  Release codename embedded in binary metadata (default: Ivysaur)
  --target <goos/goarch>
                      Build one target. May be repeated. Defaults to linux/darwin/windows amd64+arm64.
  -h, --help          Show this help

Examples:
  scripts/build-release.sh --version v0.2.0 --codename Ivysaur
  scripts/build-release.sh --target darwin/arm64 --target windows/amd64
EOF
}

log() {
  printf '[build-release] %s\n' "$*"
}

die() {
  printf '[build-release][error] %s\n' "$*" >&2
  exit 1
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
      zip -qr "${DIST_DIR}/${archive_base}.zip" .
    else
      tar -czf "${DIST_DIR}/${archive_base}.tar.gz" .
    fi
  )
}

build_target() {
  local target="$1"
  local goos="${target%/*}"
  local goarch="${target#*/}"

  [[ -n "$goos" && -n "$goarch" && "$goos" != "$goarch" ]] || die "invalid target: $target"

  local target_name="omnidex-${VERSION}-${goos}-${goarch}"
  local commit build_date ldflags
  commit="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || true)"
  build_date="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  ldflags="-X github.com/gryph/omnidex/internal/version.Version=${VERSION} -X github.com/gryph/omnidex/internal/version.Codename=${CODENAME} -X github.com/gryph/omnidex/internal/version.Commit=${commit} -X github.com/gryph/omnidex/internal/version.Date=${build_date}"
  local target_dir="${DIST_DIR}/${target_name}"
  rm -rf "$target_dir"
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
      cd "$REPO_ROOT"
      CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$ldflags" -o "${target_dir}/bin/${name}${ext}" "$pkg"
    )
  done

  cp -a "${REPO_ROOT}/README.md" "${target_dir}/README.md"
  cp -a "${REPO_ROOT}/LICENSE" "${target_dir}/LICENSE"
  if [[ -f "${REPO_ROOT}/CHANGELOG.md" ]]; then
    cp -a "${REPO_ROOT}/CHANGELOG.md" "${target_dir}/CHANGELOG.md"
  fi
  if [[ -f "${REPO_ROOT}/agent_aliases.sh" && "$goos" != "windows" ]]; then
    cp -a "${REPO_ROOT}/agent_aliases.sh" "${target_dir}/agent_aliases.sh"
  fi

  archive_target "$target_dir" "$target_name" "$goos"
}

main() {
  parse_args "$@"

  command -v go >/dev/null 2>&1 || die "go is required"
  command -v tar >/dev/null 2>&1 || die "tar is required"
  if printf '%s\n' "${TARGETS[@]}" | grep -q '^windows/'; then
    command -v zip >/dev/null 2>&1 || die "zip is required for Windows archives"
  fi

  if [[ "$DIST_DIR" != /* ]]; then
    DIST_DIR="${REPO_ROOT}/${DIST_DIR#./}"
  fi
  mkdir -p "$DIST_DIR"

  local target
  for target in "${TARGETS[@]}"; do
    build_target "$target"
  done

  (
    cd "$DIST_DIR"
    rm -f SHA256SUMS
    artifacts=()
    for artifact in omnidex-*.tar.gz omnidex-*.zip; do
      [[ -f "$artifact" ]] || continue
      artifacts+=("$artifact")
    done
    if ((${#artifacts[@]} > 0)); then
      if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${artifacts[@]}" > SHA256SUMS
      elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${artifacts[@]}" > SHA256SUMS
      else
        die "sha256sum or shasum is required"
      fi
    fi
  )
  log "release artifacts written to ${DIST_DIR}"
}

main "$@"
