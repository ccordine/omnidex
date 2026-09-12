#!/usr/bin/env bash

log() {
  printf '[build-release] %s\n' "$*"
}

die() {
  printf '[build-release][error] %s\n' "$*" >&2
  exit 1
}

validate_release_inputs() {
  ((${#VERSION} <= 64)) || die "release version exceeds 64 characters"
  [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9]+)*$ ]] || die "invalid release version: $VERSION"
  ((${#CODENAME} <= 64)) || die "release codename exceeds 64 characters"
  [[ "$CODENAME" =~ ^[A-Za-z][A-Za-z0-9_-]{0,63}$ ]] || die "invalid release codename: $CODENAME"
  ((${#TARGETS[@]} > 0)) || die "at least one release target is required"
  local target prior
  local validated=()
  for target in "${TARGETS[@]}"; do
    case "$target" in
      linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64|windows/arm64) ;;
      *) die "unsupported release target: $target" ;;
    esac
    for prior in "${validated[@]}"; do
      [[ "$target" != "$prior" ]] || die "duplicate release target: $target"
    done
    validated+=("$target")
  done
}

validate_release_cgo_targets() {
  local host_goos host_goarch host_target cc target
  host_goos="$(go env GOOS)"
  host_goarch="$(go env GOARCH)"
  host_target="${host_goos}/${host_goarch}"
  [[ "$host_target" =~ ^[a-z0-9]+/[a-z0-9]+$ ]] || die "native Go release target is invalid"
  cc="$(go env CC)"
  [[ -n "$cc" && "$cc" != *[[:space:]]* ]] || die "native CGO compiler is invalid: $cc"
  command -v "$cc" >/dev/null 2>&1 || die "native CGO compiler is unavailable: $cc"
  for target in "$@"; do
    [[ "$target" == "$host_target" ]] ||
      die "CGO release cross-compilation is unsupported: target $target requires a native $target release host"
  done
}

validate_dist_dir() {
  [[ -n "$DIST_DIR" && "$DIST_DIR" != "/" ]] || die "distribution directory must be explicit and non-root"
  if [[ "$DIST_DIR" != /* ]]; then
    DIST_DIR="${REPO_ROOT}/${DIST_DIR#./}"
  fi
  local parent name
  parent="$(dirname "$DIST_DIR")"
  name="$(basename "$DIST_DIR")"
  [[ "$name" != "." && "$name" != ".." ]] || die "distribution directory name is unsafe"
  [[ -d "$parent" ]] || die "distribution directory parent is unavailable"
  parent="$(cd "$parent" && pwd -P)"
  DIST_DIR="${parent}/${name}"
  [[ ! -L "$DIST_DIR" ]] || die "distribution directory must not be a symlink"
  [[ ! -e "$DIST_DIR" || -d "$DIST_DIR" ]] || die "distribution directory is not a directory"
  [[ "$DIST_DIR" != "$REPO_ROOT" ]] || die "distribution directory must not be the repository root"
}
