#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

OUTPUT=""
OUTPUT_SET=0
BUILD_PKG="./cmd/core"
WITH_RACE=0
VERBOSE=0
GOOS_VALUE=""
GOARCH_VALUE=""
BUILD_TAGS=""
LD_FLAGS=""

usage() {
  cat <<'EOF'
Usage:
  scripts/build-core.sh [options]

Options:
  -o, --output <path>   Output binary path (default: package-specific under ./bin)
  -p, --package <path>  Build package: ./cmd/core, ./cmd/cli, or ./cmd/omni
  --race                Build with Go race detector
  --goos <value>        Override GOOS for cross-compilation
  --goarch <value>      Override GOARCH for cross-compilation
  --tags <value>        Comma-separated Go build tags
  --ldflags <value>     Go linker flags
  -v, --verbose         Print build command before running
  -h, --help            Show this help
EOF
}

log() {
  printf '[build-core] %s\n' "$*"
}

die() {
  printf '[build-core][error] %s\n' "$*" >&2
  exit 1
}

source "${SCRIPT_DIR}/managed-checkout-lib.sh"

parse_args() {
  while (($# > 0)); do
    case "$1" in
      -o|--output)
        (($# >= 2)) || die "$1 requires a value"
        OUTPUT="$2"
        OUTPUT_SET=1
        shift 2
        ;;
      -p|--package)
        (($# >= 2)) || die "$1 requires a value"
        BUILD_PKG="$2"
        shift 2
        ;;
      --race)
        WITH_RACE=1
        shift
        ;;
      --goos)
        (($# >= 2)) || die "--goos requires a value"
        GOOS_VALUE="$2"
        shift 2
        ;;
      --goarch)
        (($# >= 2)) || die "--goarch requires a value"
        GOARCH_VALUE="$2"
        shift 2
        ;;
      --tags)
        (($# >= 2)) || die "--tags requires a value"
        BUILD_TAGS="$2"
        shift 2
        ;;
      --ldflags)
        (($# >= 2)) || die "--ldflags requires a value"
        LD_FLAGS="$2"
        shift 2
        ;;
      -v|--verbose)
        VERBOSE=1
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

parse_args "$@"

case "${BUILD_PKG}" in
  ./cmd/core|./cmd/cli|./cmd/omni) ;;
  *) die "unsupported Omnidex binary package: ${BUILD_PKG}" ;;
esac

if ((OUTPUT_SET)) && [[ -z "${OUTPUT}" ]]; then
  die "output path cannot be empty"
fi
if ((OUTPUT_SET == 0)); then
  case "${BUILD_PKG}" in
    ./cmd/core) OUTPUT="${REPO_ROOT}/bin/agent-core" ;;
    ./cmd/cli) OUTPUT="${REPO_ROOT}/bin/agent-cli" ;;
    ./cmd/omni) OUTPUT="${REPO_ROOT}/bin/omni" ;;
  esac
fi

if ! command -v go >/dev/null 2>&1; then
  die "go is required but was not found in PATH"
fi

managed_checkout_export_build_commit "${REPO_ROOT}"
if [[ "${BUILD_PKG}" == "./cmd/core" ]]; then
  "${SCRIPT_DIR}/build-ui.sh"
fi

if [[ "${OUTPUT}" != /* ]]; then
  OUTPUT="${REPO_ROOT}/${OUTPUT#./}"
fi

mkdir -p "$(dirname "${OUTPUT}")"
rm -f "${OUTPUT}"

required_ld_flags="-X github.com/gryph/omnidex/internal/version.Commit=${OMNIDEX_COMMIT}"
if [[ -n "${LD_FLAGS}" ]]; then
  required_ld_flags="${LD_FLAGS} ${required_ld_flags}"
fi

build_cmd=(go build -trimpath -ldflags "${required_ld_flags}" -o "${OUTPUT}")
if ((WITH_RACE)); then
  build_cmd+=(-race)
fi
if [[ -n "${BUILD_TAGS}" ]]; then
  build_cmd+=(-tags "${BUILD_TAGS}")
fi
build_cmd+=("${BUILD_PKG}")

env_cmd=()
if [[ -n "${GOOS_VALUE}" ]]; then
  env_cmd+=("GOOS=${GOOS_VALUE}")
fi
if [[ -n "${GOARCH_VALUE}" ]]; then
  env_cmd+=("GOARCH=${GOARCH_VALUE}")
fi

if ((VERBOSE)); then
  log "repo=${REPO_ROOT}"
  if ((${#env_cmd[@]} > 0)); then
    log "env: ${env_cmd[*]}"
  fi
  log "cmd: ${build_cmd[*]}"
fi

(
  cd "${REPO_ROOT}"
  if ((${#env_cmd[@]} > 0)); then
    env "${env_cmd[@]}" "${build_cmd[@]}"
  else
    "${build_cmd[@]}"
  fi
)

host_goos="$(go env GOOS)"
host_goarch="$(go env GOARCH)"
target_goos="${GOOS_VALUE:-${host_goos}}"
target_goarch="${GOARCH_VALUE:-${host_goarch}}"
verification_interface="metadata"
if [[ "${target_goos}" == "${host_goos}" && "${target_goarch}" == "${host_goarch}" ]]; then
  case "${BUILD_PKG}" in
    ./cmd/core) verification_interface="core" ;;
    ./cmd/cli|./cmd/omni) verification_interface="json" ;;
  esac
fi
managed_checkout_verify_binary_commit "${OUTPUT}" "${OMNIDEX_COMMIT}" "${verification_interface}"

log "built ${OUTPUT}"
