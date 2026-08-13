#!/usr/bin/env bash

resolve_compose_cmd() {
  if command_exists docker && compose_docker version >/dev/null 2>&1; then
    printf '%s\n' "docker compose"
    return
  fi
  if command_exists docker-compose; then
    [[ -z "${DOCKER_CONTEXT_NAME}" ]] ||
      die "DOCKER_CONTEXT requires the docker compose plugin"
    printf '%s\n' "docker-compose"
    return
  fi
  die "docker compose is required but was not found"
}

validate_compose_identity() {
  local label="$1" value="$2"
  [[ -z "${value}" || "${value}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] ||
    die "${label} contains unsupported characters"
}

compose_docker() {
  if [[ -n "${DOCKER_CONTEXT_NAME}" ]]; then
    env "DOCKER_CONTEXT=${DOCKER_CONTEXT_NAME}" docker compose "$@"
    return
  fi
  docker compose "$@"
}

compose_command_array() {
  local compose_cmd="$1"
  local -n output="$2"
  if [[ "${compose_cmd}" == "docker compose" ]]; then
    output=(compose_docker)
  else
    read -r -a output <<<"${compose_cmd}"
  fi
  [[ -z "${COMPOSE_PROJECT}" ]] || output+=(-p "${COMPOSE_PROJECT}")
}

needs_compose_work() {
  ((HOST_ONLY == 0 && (!NO_BUILD || !NO_RESTART)))
}

compose_build() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4"
  if ((NO_BUILD)); then
    log "skipping docker compose build (--no-build)"
    return 0
  fi
  local -a cmd=()
  compose_command_array "${compose_cmd}" cmd
  [[ -z "${compose_file}" ]] || cmd+=(-f "${compose_file}")
  cmd+=(build --pull)
  ((NO_CACHE == 0)) || cmd+=(--no-cache)
  cmd+=("${service}")
  log "rebuilding image for service ${service}"
  (cd "${repo_dir}" && "${cmd[@]}")
}

compose_restart() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4"
  if ((NO_RESTART)); then
    log "skipping docker compose up (--no-restart)"
    return 0
  fi
  local -a cmd=()
  compose_command_array "${compose_cmd}" cmd
  [[ -z "${compose_file}" ]] || cmd+=(-f "${compose_file}")
  cmd+=(up -d --remove-orphans "${service}")
  log "restarting service ${service}"
  (cd "${repo_dir}" && "${cmd[@]}")
}

host_bridge_unit_file() {
  printf '%s\n' "${HOME}/.config/systemd/user/omni-host-bridge.service"
}

host_bridge_omni_from_unit() {
  local unit="$1" exec_start
  exec_start="$(sed -n 's/^ExecStart=//p' "${unit}" | head -n 1)"
  case "${exec_start}" in
    *" host serve") printf '%s\n' "${exec_start% host serve}" ;;
    *) printf '%s\n' "" ;;
  esac
}

refresh_host_bridge_binary_for_unit() {
  local repo_dir="$1" unit="$2"
  local built_omni="${repo_dir}/bin/omni"
  local service_omni service_dir tmp
  service_omni="$(host_bridge_omni_from_unit "${unit}")"
  if [[ -z "${service_omni}" ]]; then
    warn "could not parse host bridge ExecStart from ${unit}"
    return 0
  fi
  [[ "${service_omni}" != "${built_omni}" ]] || return 0
  warn "host bridge unit uses a different binary: ${service_omni}"
  case "${service_omni}" in
    "${HOME}"/*) ;;
    *)
      warn "not refreshing ${service_omni}; it is outside ${HOME}"
      warn "reinstall the bridge with: ${built_omni} host service install --omni ${built_omni}"
      return 0
      ;;
  esac
  service_dir="$(dirname "${service_omni}")"
  mkdir -p "${service_dir}"
  log "refreshing host bridge binary at ${service_omni}"
  tmp="${service_omni}.new.$$"
  install -m 0755 "${built_omni}" "${tmp}"
  mv -f "${tmp}" "${service_omni}"
  if [[ -x "${repo_dir}/bin/agent-core" ]]; then
    tmp="${service_dir}/agent-core.new.$$"
    install -m 0755 "${repo_dir}/bin/agent-core" "${tmp}"
    mv -f "${tmp}" "${service_dir}/agent-core"
  fi
  if [[ -x "${repo_dir}/bin/agent-cli" ]]; then
    tmp="${service_dir}/agent-cli.new.$$"
    install -m 0755 "${repo_dir}/bin/agent-cli" "${tmp}"
    mv -f "${tmp}" "${service_dir}/agent-cli"
    ln -sfn agent-cli "${service_dir}/acli"
  fi
}

restart_host_bridge() {
  local repo_dir="$1" unit
  local omni="${repo_dir}/bin/omni"
  if ((NO_HOST_RESTART)); then
    log "skipping host bridge restart (--no-host-restart)"
    return 0
  fi
  [[ -x "${omni}" ]] || die "updated bin/omni is not executable"
  unit="$(host_bridge_unit_file)"
  if [[ ! -f "${unit}" ]]; then
    log "host bridge service not installed; skipping restart (run: ${omni} host service install)"
    return 0
  fi
  refresh_host_bridge_binary_for_unit "${repo_dir}" "${unit}"
  log "restarting host bridge (omni-host-bridge)"
  "${omni}" host service restart || die "host bridge restart failed; check: ${omni} host service status"
  log "host bridge restarted"
}
