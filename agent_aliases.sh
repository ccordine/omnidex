#!/usr/bin/env bash
# Source this file to get convenient helpers for Omnidex (short name: omni).
# Example:
#   source /absolute/path/to/omnidex/agent_aliases.sh

_agent_aliases_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${OMNIDEX_DIR:-}" \
   || ! -d "${OMNIDEX_DIR}" \
   || ! -f "${OMNIDEX_DIR}/go.mod" ]]; then
  OMNIDEX_DIR="${_agent_aliases_script_dir}"
fi
unset _agent_aliases_script_dir

_agent_cli_cmd() {
  local caller_cwd="${PWD}"

  if [[ -x "${OMNIDEX_DIR}/bin/agent-cli" ]]; then
    OMNI_INVOKE_CWD="${caller_cwd}" "${OMNIDEX_DIR}/bin/agent-cli" "$@"
    return $?
  fi

  if command -v agent-cli >/dev/null 2>&1; then
    OMNI_INVOKE_CWD="${caller_cwd}" command agent-cli "$@"
    return $?
  fi

  (cd "${OMNIDEX_DIR}" && OMNI_INVOKE_CWD="${caller_cwd}" go run ./cmd/cli "$@")
}

_omni_cmd() {
  local caller_cwd="${PWD}"

  if [[ -x "${OMNIDEX_DIR}/bin/omni" ]]; then
    OMNI_INVOKE_CWD="${caller_cwd}" "${OMNIDEX_DIR}/bin/omni" "$@"
    return $?
  fi

  if [[ "${OMNIDEX_USE_SYSTEM_OMNI:-0}" != "1" ]]; then
    (cd "${OMNIDEX_DIR}" && OMNI_INVOKE_CWD="${caller_cwd}" go run ./cmd/omni "$@")
    return $?
  fi

  if omni_bin="$(type -P omni 2>/dev/null)"; then
    OMNI_INVOKE_CWD="${caller_cwd}" "${omni_bin}" "$@"
    return $?
  fi

  (cd "${OMNIDEX_DIR}" && OMNI_INVOKE_CWD="${caller_cwd}" go run ./cmd/omni "$@")
}

# Canonical deterministic Omnidex CLI
omni() { _omni_cmd "$@"; }
omnidex() { _omni_cmd "$@"; }

# Queue/API CLI passthrough
acli() { _agent_cli_cmd "$@"; }

# Core URL helper
asetcore() {
  if [[ -z "${1:-}" ]]; then
    echo "usage: asetcore <url>"
    return 1
  fi
  export CORE_URL="$1"
  echo "CORE_URL=${CORE_URL}"
}

# Host dependency bootstrap
asetupdeps() {
  (cd "${OMNIDEX_DIR}" && ./scripts/setup-host-deps.sh "$@")
}

aupdate() {
  (cd "${OMNIDEX_DIR}" && ./update.sh "$@")
}

# Enqueue helpers
# usage: aq "instruction"
aq()   { _agent_cli_cmd enqueue --pipeline assistant --web auto --workspace auto "$@"; }
aqf()  { _agent_cli_cmd enqueue --pipeline assistant --reasoning fast --web auto --workspace auto "$@"; }
aqd()  { _agent_cli_cmd enqueue --pipeline assistant --reasoning deep --web auto --workspace auto "$@"; }

# Pipeline variants
achat() { _agent_cli_cmd enqueue --pipeline chat --web auto --workspace auto "$@"; }
aqarch() { _agent_cli_cmd enqueue --profile architect --pipeline assistant "$@"; }
achatarch() { _agent_cli_cmd chat --profile architect "$@"; }
achatrepl() { _agent_cli_cmd chat "$@"; }
astro() { _agent_cli_cmd enqueue --pipeline story --web auto --workspace auto "$@"; }

# Job inspection
alist() { _agent_cli_cmd list "$@"; }
arun()  { _agent_cli_cmd list --status running "$@"; }
awaiting() { _agent_cli_cmd list --status waiting_input "$@"; }
ashow() { _agent_cli_cmd show "$@"; }
awatch() { _agent_cli_cmd watch "$@"; }
awv() { _agent_cli_cmd watch --interval 2s --verbose --max-chars 1600 "$@"; }

# Feedback and memory
afb()       { _agent_cli_cmd feedback "$@"; }
aint()      { _agent_cli_cmd interrupt "$@"; }
areplan()   { _agent_cli_cmd replan "$@"; }
acont()     { _agent_cli_cmd continue "$@"; }
acancel()   { _agent_cli_cmd cancel "$@"; }
aremember() { _agent_cli_cmd remember "$@"; }
aingest()   { _agent_cli_cmd ingest "$@"; }
amediaindex() { _agent_cli_cmd media-index "$@"; }
amediasearch() { _agent_cli_cmd media-search "$@"; }
abrowserscan() { _agent_cli_cmd browser-scan "$@"; }
ascreenread() { _agent_cli_cmd screen-read "$@"; }
aresearch() { _agent_cli_cmd research "$@"; }
aperms() { _agent_cli_cmd permissions "$@"; }
anotes() { _agent_cli_cmd audio-notes "$@"; }

# Latest job helpers
alast_id() {
  _agent_cli_cmd list --limit 1 | awk '/^#/ {gsub("#", "", $1); print $1; exit}'
}

alast() {
  local id
  id="$(alast_id)"
  if [[ -z "${id}" ]]; then
    echo "no jobs found"
    return 1
  fi
  echo "${id}"
}

aslatest() {
  local id
  id="$(alast_id)"
  if [[ -z "${id}" ]]; then
    echo "no jobs found"
    return 1
  fi
  _agent_cli_cmd show "${id}"
}

awlatest() {
  local id
  id="$(alast_id)"
  if [[ -z "${id}" ]]; then
    echo "no jobs found"
    return 1
  fi
  _agent_cli_cmd watch "${id}"
}

awlatestv() {
  local id
  id="$(alast_id)"
  if [[ -z "${id}" ]]; then
    echo "no jobs found"
    return 1
  fi
  _agent_cli_cmd watch --interval 2s --verbose --max-chars 1600 "${id}"
}
