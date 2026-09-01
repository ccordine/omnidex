#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() {
  printf '[up][error] %s\n' "$*" >&2
  exit 1
}

source "${SCRIPT_DIR}/scripts/managed-checkout-lib.sh"

start_ollama() {
  local llm_provider embedding_provider
  managed_checkout_require_env_key "${SCRIPT_DIR}/.env" "LLM_PROVIDER"
  managed_checkout_require_env_key "${SCRIPT_DIR}/.env" "EMBEDDING_PROVIDER"
  llm_provider="$(managed_checkout_env_value "${SCRIPT_DIR}/.env" "LLM_PROVIDER")"
  embedding_provider="$(managed_checkout_env_value "${SCRIPT_DIR}/.env" "EMBEDDING_PROVIDER")"
  if [[ "${llm_provider,,}" != "ollama" && "${embedding_provider,,}" != "ollama" ]]; then
    return 0
  fi

  if curl -fsS --max-time 3 http://127.0.0.1:11434/api/tags >/dev/null; then
    return 0
  fi
  if systemctl --user cat ollama.service >/dev/null 2>&1; then
    systemctl --user start ollama.service || die "failed to start user ollama.service"
  elif systemctl cat ollama.service >/dev/null 2>&1; then
    sudo systemctl start ollama.service || die "failed to start system ollama.service"
  else
    die "Ollama is required but no ollama.service is installed"
  fi
  curl -fsS --retry 20 --retry-delay 1 --retry-all-errors --max-time 3 \
    http://127.0.0.1:11434/api/tags >/dev/null ||
    die "Ollama did not become reachable at http://127.0.0.1:11434"
}

start_ollama
"${SCRIPT_DIR}/scripts/compose-deployment.sh" up "$@"
