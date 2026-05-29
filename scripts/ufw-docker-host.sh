#!/usr/bin/env bash
set -euo pipefail

# Allow Docker bridge networks to reach host services (Ollama, omni host bridge).
# Common fix on Arch Linux when UFW is active and core in Docker cannot reach the host.

OLLAMA_PORT="${OLLAMA_PORT:-11434}"
BRIDGE_PORT="${BRIDGE_PORT:-8091}"
DOCKER_CIDR="${DOCKER_CIDR:-172.16.0.0/12}"

usage() {
  cat <<EOF
Usage: scripts/ufw-docker-host.sh [options]

Adds UFW rules so Docker containers can reach host Ollama and the omni host bridge.
Typical Arch Linux fix when probes time out from inside the core container.

Options:
  --ollama-port <N>   Ollama port (default: 11434)
  --bridge-port <N>   Host bridge port (default: 8091)
  --cidr <CIDR>       Docker network range (default: 172.16.0.0/12)
  --dry-run           Print commands without running them
  -h, --help          Show this help

After running, restart core and verify:
  docker compose exec core wget -qO- --timeout=5 http://host.docker.internal:${OLLAMA_PORT}/api/tags
  docker compose exec core wget -qO- --timeout=5 http://host.docker.internal:${BRIDGE_PORT}/healthz
EOF
}

DRY_RUN=0

run_cmd() {
  if ((DRY_RUN)); then
    printf '+'
    for token in "$@"; do
      printf ' %q' "$token"
    done
    printf '\n'
    return 0
  fi
  "$@"
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --ollama-port)
        OLLAMA_PORT="$2"
        shift 2
        ;;
      --bridge-port)
        BRIDGE_PORT="$2"
        shift 2
        ;;
      --cidr)
        DOCKER_CIDR="$2"
        shift 2
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *)
        echo "unknown option: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
  done
}

main() {
  parse_args "$@"

  if ! command -v ufw >/dev/null 2>&1; then
    echo "ufw not found; this script is for hosts using UFW (common on Arch)." >&2
    exit 1
  fi

  echo "[ufw-docker-host] allowing ${DOCKER_CIDR} -> host ports ${OLLAMA_PORT}, ${BRIDGE_PORT}"
  run_cmd sudo ufw allow from "${DOCKER_CIDR}" to any port "${OLLAMA_PORT}" proto tcp comment 'Ollama from Docker'
  run_cmd sudo ufw allow from "${DOCKER_CIDR}" to any port "${BRIDGE_PORT}" proto tcp comment 'Omni host bridge from Docker'
  run_cmd sudo ufw status | grep -E "${OLLAMA_PORT}|${BRIDGE_PORT}|172\.16" || true
  echo "[ufw-docker-host] done. Re-test from core container (see --help)."
}

main "$@"
