#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_ROOT="${REPO_ROOT}/internal/api/web"

die() {
  printf '[build-ui][error] %s\n' "$*" >&2
  exit 1
}

run_npm_install() {
  npm ci --include=dev --no-audit --no-fund
}

ensure_esbuild_binary() {
  local esbuild_bin="${WEB_ROOT}/node_modules/esbuild/bin/esbuild"
  local esbuild_install_dir="${WEB_ROOT}/node_modules/esbuild"

  if [[ ! -d "${esbuild_install_dir}" ]]; then
    return 1
  fi
  if [[ ! -f "${esbuild_bin}" || ! -x "${esbuild_bin}" ]]; then
    return 1
  fi
  return 0
}

command -v node >/dev/null 2>&1 || die "node is required to build the embedded GUI"
command -v npm >/dev/null 2>&1 || die "npm is required to build the embedded GUI"
[[ -f "${WEB_ROOT}/package.json" ]] || die "GUI package.json is missing"
[[ -f "${WEB_ROOT}/package-lock.json" ]] || die "GUI package-lock.json is required for a reproducible build"

(
  cd "${WEB_ROOT}"
  run_npm_install || die "npm ci failed; a complete local dependency layout is required before build"
  ensure_esbuild_binary || die "npm ci completed without the local esbuild binary"

  npm run build
)

[[ -s "${WEB_ROOT}/dist/index.html" ]] || die "GUI build did not produce dist/index.html"
[[ -s "${WEB_ROOT}/dist/.vite/manifest.json" ]] || die "GUI build did not produce the Vite manifest"
printf '[build-ui] built %s\n' "${WEB_ROOT}/dist"
