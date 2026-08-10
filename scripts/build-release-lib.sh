#!/usr/bin/env bash

log() {
  printf '[build-release] %s\n' "$*"
}

die() {
  printf '[build-release][error] %s\n' "$*" >&2
  exit 1
}

sha256_file() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{print $1}'
  else
    die "sha256sum or shasum is required"
  fi
}

write_migration_manifest() {
  local source_dir="$1"
  local target_dir="$2"
  local manifest="${target_dir}/SHA256SUMS"
  local migration filename digest previous=""
  local migrations=()
  [[ -d "$source_dir" && ! -L "$source_dir" ]] || die "migration source must be a real directory"
  [[ -d "$target_dir" && ! -L "$target_dir" ]] || die "migration target must be a real directory"
  while IFS= read -r -d '' migration; do
    filename="${migration##*/}"
    [[ -f "$migration" && ! -L "$migration" ]] || die "unregistered migration directory entry: $filename"
    [[ "$filename" =~ ^[0-9]{3}_[A-Za-z0-9_]+\.sql$ ]] || die "invalid migration filename: $filename"
    migrations+=("$migration")
  done < <(find "$source_dir" -mindepth 1 -maxdepth 1 -print0 | LC_ALL=C sort -z)
  ((${#migrations[@]} > 0)) || die "release migration set is empty"
  : > "$manifest"
  for migration in "${migrations[@]}"; do
    filename="${migration##*/}"
    [[ -z "$previous" || "$filename" > "$previous" ]] || die "migration filenames are not strictly ordered"
    digest="$(sha256_file "$migration")"
    [[ "$digest" =~ ^[0-9a-f]{64}$ ]] || die "migration digest is invalid: $filename"
    printf '%s  %s\n' "$digest" "$filename" >> "$manifest"
    previous="$filename"
  done
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

validate_dist_dir() {
  [[ -n "$DIST_DIR" && "$DIST_DIR" != "/" ]] || die "distribution directory must be explicit and non-root"
  if [[ "$DIST_DIR" != /* ]]; then
    DIST_DIR="${REPO_ROOT}/${DIST_DIR#./}"
  fi
  local parent name
  parent="$(dirname "$DIST_DIR")"
  name="$(basename "$DIST_DIR")"
  [[ "$name" != "." && "$name" != ".." ]] || die "distribution directory name is unsafe"
  [[ -d "$parent" && ! -L "$parent" ]] || die "distribution directory parent is unavailable or a symlink"
  parent="$(cd "$parent" && pwd -P)"
  DIST_DIR="${parent}/${name}"
  if [[ -e "$DIST_DIR" ]]; then
    [[ -d "$DIST_DIR" && ! -L "$DIST_DIR" ]] || die "distribution directory must be a real directory"
    DIST_DIR="$(cd "$DIST_DIR" && pwd -P)"
  fi
  [[ -n "$DIST_DIR" && "$DIST_DIR" != "/" ]] || die "distribution directory resolved to root"
  [[ "$DIST_DIR" != "$REPO_ROOT" && "$DIST_DIR" == "$REPO_ROOT"/* ]] ||
    die "distribution directory must be a strict repository descendant"
  local relative="${DIST_DIR#"$REPO_ROOT"/}" cursor="$REPO_ROOT" component prefix=""
  local components=()
  IFS='/' read -r -a components <<< "$relative"
  for component in "${components[@]}"; do
    cursor="${cursor}/${component}"
    prefix="${prefix:+${prefix}/}${component}"
    [[ ! -L "$cursor" ]] || die "distribution path contains a symlink: $component"
    [[ -z "$(git -C "$REPO_ROOT" ls-files -- "$prefix")" ]] ||
      die "distribution path enters tracked source: $prefix"
  done
}

create_dist_dir() {
  mkdir -p "$DIST_DIR"
  [[ -d "$DIST_DIR" && ! -L "$DIST_DIR" && "$(cd "$DIST_DIR" && pwd -P)" == "$DIST_DIR" ]] ||
    die "distribution directory changed after validation"
}

verified_target_dir() {
  local target_name="$1"
  [[ "$target_name" =~ ^omnidex-v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9]+)*-(linux|darwin|windows)-(amd64|arm64)$ ]] ||
    die "unsafe release target name: $target_name"
  local target_dir="${DIST_DIR}/${target_name}"
  [[ "$(dirname "$target_dir")" == "$DIST_DIR" && "$target_dir" == "$DIST_DIR"/* ]] ||
    die "release target is not a strict distribution child"
  [[ ! -L "$target_dir" ]] || die "release target must not be a symlink"
  printf '%s\n' "$target_dir"
}

assert_repository_matches_snapshot() {
  local repository="$1" commit="$2" ignored_dist="$3"
  [[ "$(git -C "$repository" rev-parse HEAD 2>/dev/null || true)" == "$commit" ]] ||
    die "repository HEAD changed during release build"
  git -C "$repository" diff --quiet --ignore-submodules -- ||
    die "tracked source changed during release build"
  git -C "$repository" diff --cached --quiet --ignore-submodules -- ||
    die "staged source changed during release build"
  local path ignored_relative="${ignored_dist#"$repository"/}"
  while IFS= read -r -d '' path; do
    [[ "$path" == "$ignored_relative" || "$path" == "$ignored_relative"/* ]] && continue
    die "untracked source appeared during release build: $path"
  done < <(git -C "$repository" ls-files --others --exclude-standard -z)
}

write_source_manifest() {
  local source_dir="$1" manifest="$2" entry relative digest
  [[ -d "$source_dir" && ! -L "$source_dir" ]] || die "release source tree is unavailable"
  : > "$manifest"
  while IFS= read -r -d '' entry; do
    relative="${entry#"$source_dir"/}"
    [[ "$relative" != *$'\n'* && "$relative" != *$'\r'* ]] ||
      die "release source path contains a line break"
    if [[ -d "$entry" && ! -L "$entry" ]]; then
      printf 'directory  %s\n' "$relative" >> "$manifest"
    elif [[ -f "$entry" && ! -L "$entry" ]]; then
      digest="$(sha256_file "$entry")"
      printf 'file %s  %s\n' "$digest" "$relative" >> "$manifest"
    else
      die "release source contains an unsupported entry: $relative"
    fi
  done < <(find "$source_dir" -mindepth 1 -print0 | LC_ALL=C sort -z)
}
