#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

OUTPUT="${REPO_ROOT}/bin/agent-core"
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
  -o, --output <path>   Output binary path (default: ./bin/agent-core)
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

parse_args() {
  while (($# > 0)); do
    case "$1" in
      -o|--output)
        (($# >= 2)) || die "$1 requires a value"
        OUTPUT="$2"
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

if ! command -v go >/dev/null 2>&1; then
  die "go is required but was not found in PATH"
fi

if [[ -z "${OUTPUT}" ]]; then
  die "output path cannot be empty"
fi

if [[ "${OUTPUT}" != /* ]]; then
  OUTPUT="${REPO_ROOT}/${OUTPUT#./}"
fi

mkdir -p "$(dirname "${OUTPUT}")"

build_cmd=(go build -o "${OUTPUT}")
if ((WITH_RACE)); then
  build_cmd+=(-race)
fi
if [[ -n "${BUILD_TAGS}" ]]; then
  build_cmd+=(-tags "${BUILD_TAGS}")
fi
if [[ -n "${LD_FLAGS}" ]]; then
  build_cmd+=(-ldflags "${LD_FLAGS}")
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

log "built ${OUTPUT}"
