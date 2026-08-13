#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_ROOT="${REPO_ROOT}/internal/api/web"

die() {
  printf '[build-ui][error] %s\n' "$*" >&2
  exit 1
}

command -v node >/dev/null 2>&1 || die "node is required to build the embedded GUI"
command -v npm >/dev/null 2>&1 || die "npm is required to build the embedded GUI"
[[ -f "${WEB_ROOT}/package.json" ]] || die "GUI package.json is missing"
[[ -f "${WEB_ROOT}/package-lock.json" ]] || die "GUI package-lock.json is required for a reproducible build"

(
  cd "${WEB_ROOT}"
  npm ci --no-audit --no-fund
  npm run build
)

[[ -s "${WEB_ROOT}/dist/index.html" ]] || die "GUI build did not produce dist/index.html"
[[ -s "${WEB_ROOT}/dist/.vite/manifest.json" ]] || die "GUI build did not produce the Vite manifest"
printf '[build-ui] built %s\n' "${WEB_ROOT}/dist"
