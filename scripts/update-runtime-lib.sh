#!/usr/bin/env bash

resolve_compose_cmd() {
  runtime_require_rootful_docker_context
  command_exists docker || die "the Docker Compose plugin is required but was not found"
  runtime_require_rootful_docker_daemon
  if compose_docker version >/dev/null 2>&1; then
    printf '%s\n' "docker compose"
    return
  fi
  die "the Docker Compose plugin is required but was not found"
}

runtime_require_rootful_docker_context() {
  [[ "${DOCKER_CONTEXT_NAME:-}" == "default" ]] ||
    die "DOCKER_CONTEXT must be default; rootless Docker is unsupported"
}

runtime_reject_managed_docker_routing_keys() {
  local file="$1" key
  [[ -f "${file}" && ! -L "${file}" ]] || die "managed .env must be a regular file"
  for key in \
    DOCKER_SOCKET_PATH DOCKER_HOST DOCKER_CONFIG DOCKER_CERT_PATH \
    DOCKER_TLS DOCKER_TLS_VERIFY BUILDKIT_HOST BUILDKIT_TLS_SERVER_NAME \
    BUILDKIT_TLS_CACERT BUILDKIT_TLS_CERT BUILDKIT_TLS_KEY \
    BUILDX_BUILDER BUILDX_CONFIG; do
    if awk -v wanted="${key}" '
      index($0, wanted "=") == 1 { found=1 }
      END { exit found ? 0 : 1 }
    ' "${file}"; then
      die "managed .env must not define ${key}; Docker authority is invariant"
    fi
  done
}

runtime_rootful_docker() {
  env -u DOCKER_HOST -u DOCKER_CONFIG -u DOCKER_CERT_PATH -u DOCKER_TLS \
    -u DOCKER_TLS_VERIFY -u BUILDKIT_HOST -u BUILDKIT_TLS_SERVER_NAME \
    -u BUILDKIT_TLS_CACERT -u BUILDKIT_TLS_CERT -u BUILDKIT_TLS_KEY \
    -u BUILDX_BUILDER -u BUILDX_CONFIG DOCKER_CONTEXT=default docker "$@"
}

runtime_require_rootful_docker_daemon() {
  runtime_require_rootful_docker_context
  local endpoint security_options
  endpoint="$(runtime_rootful_docker context inspect default --format '{{(index .Endpoints "docker").Host}}')" ||
    die "Docker's default rootful context is unavailable"
  [[ "${endpoint}" == "unix:///var/run/docker.sock" ]] ||
    die "Docker's default context must resolve to unix:///var/run/docker.sock"
  security_options="$(runtime_rootful_docker info --format '{{json .SecurityOptions}}')" ||
    die "the default rootful Docker daemon is unavailable"
  [[ "${security_options}" == \[*\] ]] ||
    die "the default Docker daemon returned invalid security authority"
  [[ "${security_options}" != *name=rootless* ]] ||
    die "the default Docker daemon reported rootless execution authority"
}

runtime_validate_build_commit() {
  local commit="$1"
  [[ "${commit}" =~ ^[0123456789abcdef]{40}([0123456789abcdef]{24})?$ ]] ||
    die "OMNIDEX_COMMIT must be exactly 40 or 64 lowercase hexadecimal characters"
}

runtime_validate_host_id() {
  local label="$1" value="$2"
  [[ "${value}" =~ ^[1-9][0-9]{0,9}$ ]] && ((10#${value} <= 4294967294)) ||
    die "${label} must be one exact positive numeric host identity"
}

runtime_user_identity() {
  local uid="$1" gid="$2"
  runtime_validate_host_id "HOST_UID" "${uid}"
  runtime_validate_host_id "HOST_GID" "${gid}"
  printf '%s:%s\n' "${uid}" "${gid}"
}

runtime_validate_user_identity() {
  local value="$1" uid gid
  [[ "${value}" == *:* && "${value}" != *:*:* ]] ||
    die "runtime user must be one exact positive numeric UID:GID"
  uid="${value%%:*}"
  gid="${value#*:}"
  runtime_validate_host_id "HOST_UID" "${uid}"
  runtime_validate_host_id "HOST_GID" "${gid}"
}

runtime_export_compose_identity() {
  [[ -n "${COMPOSE_PROJECT:-}" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"
  validate_compose_identity "COMPOSE_PROJECT_NAME" "${COMPOSE_PROJECT}"
  runtime_validate_host_id "HOST_UID" "${HOST_UID:-}"
  runtime_validate_host_id "HOST_GID" "${HOST_GID:-}"
  export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT}"
  export HOST_UID HOST_GID
}

validate_compose_identity() {
  local label="$1" value="$2"
  [[ -z "${value}" || "${value}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] ||
    die "${label} contains unsupported characters"
}

compose_docker() {
  runtime_require_rootful_docker_context
  runtime_export_compose_identity
  runtime_rootful_docker compose "$@"
}

context_docker() {
  runtime_require_rootful_docker_context
  runtime_rootful_docker "$@"
}

compose_command_array() {
  local compose_cmd="$1"
  local -n output="$2"
	[[ "${compose_cmd}" == "docker compose" ]] || die "unsupported compose implementation"
	output=(compose_docker)
  [[ -n "${COMPOSE_PROJECT:-}" ]] || die "COMPOSE_PROJECT_NAME must be explicit and non-empty"
  validate_compose_identity "COMPOSE_PROJECT_NAME" "${COMPOSE_PROJECT}"
  output+=(-p "${COMPOSE_PROJECT}")
}

needs_compose_work() {
  ((HOST_ONLY == 0 && (!NO_BUILD || !NO_RESTART)))
}

compose_build() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4" commit="$5"
  runtime_validate_build_commit "${commit}"
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
  (
    cd "${repo_dir}"
    export OMNIDEX_COMMIT="${commit}"
    runtime_export_compose_identity
    "${cmd[@]}"
  )
}

compose_restart() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4" commit="$5"
  runtime_validate_build_commit "${commit}"
  if ((NO_RESTART)); then
    log "skipping docker compose up (--no-restart)"
    return 0
  fi
  local -a cmd=()
  compose_command_array "${compose_cmd}" cmd
  [[ -z "${compose_file}" ]] || cmd+=(-f "${compose_file}")
  cmd+=(up -d --remove-orphans --wait --wait-timeout 180 "${service}")
  log "restarting service ${service} and waiting for health"
  (
    cd "${repo_dir}"
    export OMNIDEX_COMMIT="${commit}"
    runtime_export_compose_identity
    "${cmd[@]}"
  )
}

compose_image_id() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4" commit="$5"
  runtime_validate_build_commit "${commit}"
  [[ "${service}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] ||
    die "compose service name is invalid: ${service}"
  local -a cmd=()
  compose_command_array "${compose_cmd}" cmd
  [[ -z "${compose_file}" ]] || cmd+=(-f "${compose_file}")
  cmd+=(config --images "${service}")
  local image_refs_raw image_ref image_id image_project image_service known_ref duplicate_ref
  image_refs_raw="$(
    cd "${repo_dir}"
    export OMNIDEX_COMMIT="${commit}"
    runtime_export_compose_identity
    "${cmd[@]}"
  )" || die "compose service ${service} configured image references are unavailable"
  [[ -n "${image_refs_raw}" ]] ||
    die "compose service ${service} returned no configured image references"

  local -a image_refs=() seen_refs=()
  local -A matching_ids=()
  mapfile -t image_refs <<< "${image_refs_raw}"
  for image_ref in "${image_refs[@]}"; do
    [[ -n "${image_ref}" && ${#image_ref} -le 1024 && "${image_ref}" != -* && ! "${image_ref}" =~ [[:space:]] ]] ||
      die "compose service ${service} returned an invalid configured image reference"
    duplicate_ref=0
    for known_ref in "${seen_refs[@]}"; do
      if [[ "${known_ref}" == "${image_ref}" ]]; then
        duplicate_ref=1
        break
      fi
    done
    ((duplicate_ref == 0)) || continue
    seen_refs+=("${image_ref}")

    # The selected service can depend on images that have not been pulled yet.
    # Only currently present configured references can identify the selected
    # service image; container state is intentionally irrelevant.
    if ! image_id="$(context_docker image inspect --format '{{.Id}}' "${image_ref}" 2>/dev/null)"; then
      continue
    fi
    [[ "${image_id}" =~ ^sha256:[0-9a-f]{64}$ ]] ||
      die "configured image ${image_ref} returned an invalid image identity"
    image_project="$(context_docker image inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "${image_id}")" ||
      die "configured image ${image_ref} project identity is unavailable"
    image_service="$(context_docker image inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "${image_id}")" ||
      die "configured image ${image_ref} service identity is unavailable"
    if [[ "${image_project}" == "${COMPOSE_PROJECT}" && "${image_service}" == "${service}" ]]; then
      matching_ids["${image_id}"]=1
    fi
  done

  (( ${#matching_ids[@]} == 1 )) ||
    die "compose service ${service} resolved ${#matching_ids[@]} current configured image identities for project ${COMPOSE_PROJECT}; expected exactly one"
  for image_id in "${!matching_ids[@]}"; do
    printf '%s\n' "${image_id}"
  done
}

compose_require_image_commit() {
  local image_id="$1" expected_commit="$2" expected_user="$3" image_commit image_user
  runtime_validate_build_commit "${expected_commit}"
  runtime_validate_user_identity "${expected_user}"
  [[ "${image_id}" =~ ^sha256:[0-9a-f]{64}$ ]] ||
    die "release verification requires one exact image identity"
  image_commit="$(context_docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${image_id}")"
  [[ "${image_commit}" == "${expected_commit}" ]] ||
    die "image ${image_id} has release commit ${image_commit:-<missing>}, expected ${expected_commit}"
  image_user="$(context_docker image inspect --format '{{.Config.User}}' "${image_id}")"
  [[ "${image_user}" == "${expected_user}" ]] ||
    die "image ${image_id} has runtime user ${image_user:-<missing>}, expected ${expected_user}"
  log "verified image ${image_id} release commit ${image_commit} runtime user ${image_user}"
}

compose_require_running_image() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4" expected_image="$5" expected_commit="$6" expected_user="$7"
  runtime_validate_build_commit "${expected_commit}"
  runtime_validate_user_identity "${expected_user}"
  local -a cmd=()
  compose_command_array "${compose_cmd}" cmd
  [[ -z "${compose_file}" ]] || cmd+=(-f "${compose_file}")
  cmd+=(ps -q "${service}")
  local container_id running_image running_commit running_user health_commit
  container_id="$(
    cd "${repo_dir}"
    export OMNIDEX_COMMIT="${expected_commit}"
    runtime_export_compose_identity
    "${cmd[@]}"
  )"
  [[ "${container_id}" =~ ^[0-9a-f]{12,64}$ ]] ||
    die "compose service ${service} did not resolve to one running container"
  running_image="$(context_docker inspect --type container --format '{{.Image}}' "${container_id}")"
  [[ "${running_image}" == "${expected_image}" ]] ||
    die "compose service ${service} is running image ${running_image}, expected ${expected_image}"
  running_commit="$(context_docker inspect --type container --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${container_id}")"
  [[ "${running_commit}" == "${expected_commit}" ]] ||
    die "compose service ${service} is running release commit ${running_commit:-<missing>}, expected ${expected_commit}"
  running_user="$(context_docker inspect --type container --format '{{.Config.User}}' "${container_id}")"
  [[ "${running_user}" == "${expected_user}" ]] ||
    die "compose service ${service} is running as ${running_user:-<missing>}, expected ${expected_user}"
  if ! health_commit="$(context_docker exec "${container_id}" /usr/local/bin/agent-core release:verify-running-health "${expected_commit}")"; then
    die "compose service ${service} health release identity is unreachable"
  fi
  [[ "${health_commit}" == "${expected_commit}" ]] ||
    die "compose service ${service} health reports release commit ${health_commit:-<missing>}, expected ${expected_commit}"
  log "verified running service ${service} image ${running_image} release commit ${running_commit} runtime user ${running_user}"
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
