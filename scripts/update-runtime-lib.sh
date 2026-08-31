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
  local container_id running_image running_commit running_user running_health health_commit
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
  running_health="$(context_docker inspect --type container --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${container_id}")"
  [[ "${running_health}" == "healthy" ]] ||
    die "compose service ${service} health is ${running_health:-<missing>}, expected healthy"
  health_commit="$(context_docker exec "${container_id}" /usr/local/bin/omnidex health --expect-commit "${expected_commit}")" ||
    die "compose service ${service} typed health verification failed"
  [[ "${health_commit}" == "${expected_commit}" ]] ||
    die "compose service ${service} health reports release commit ${health_commit:-<missing>}, expected ${expected_commit}"
  log "verified running service ${service} image ${running_image} release commit ${running_commit} runtime user ${running_user}"
}

compose_require_healthy_service() {
  local repo_dir="$1" compose_cmd="$2" compose_file="$3" service="$4"
  [[ "${service}" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] ||
    die "compose service name is invalid: ${service}"
  local -a base_cmd=() image_cmd=() container_cmd=() configured_images=()
  compose_command_array "${compose_cmd}" base_cmd
  [[ -z "${compose_file}" ]] || base_cmd+=(-f "${compose_file}")
  image_cmd=("${base_cmd[@]}" config --images "${service}")
  container_cmd=("${base_cmd[@]}" ps -q "${service}")
  mapfile -t configured_images < <(
    cd "${repo_dir}"
    runtime_export_compose_identity
    "${image_cmd[@]}"
  )
  (( ${#configured_images[@]} == 1 )) ||
    die "compose service ${service} resolved ${#configured_images[@]} configured images; expected exactly one"
  local configured_image="${configured_images[0]}" configured_image_id container_id
  local running_image_id running_image_ref running_project running_service running_status running_health
  [[ -n "${configured_image}" && "${configured_image}" != -* && ! "${configured_image}" =~ [[:space:]] ]] ||
    die "compose service ${service} returned an invalid configured image reference"
  configured_image_id="$(context_docker image inspect --format '{{.Id}}' "${configured_image}")" ||
    die "compose service ${service} configured image is unavailable: ${configured_image}"
  [[ "${configured_image_id}" =~ ^sha256:[0-9a-f]{64}$ ]] ||
    die "compose service ${service} configured image has invalid identity"
  container_id="$(
    cd "${repo_dir}"
    runtime_export_compose_identity
    "${container_cmd[@]}"
  )"
  [[ "${container_id}" =~ ^[0-9a-f]{12,64}$ ]] ||
    die "compose service ${service} did not resolve to one running container"
  running_image_id="$(context_docker inspect --type container --format '{{.Image}}' "${container_id}")"
  running_image_ref="$(context_docker inspect --type container --format '{{.Config.Image}}' "${container_id}")"
  running_project="$(context_docker inspect --type container --format '{{ index .Config.Labels "com.docker.compose.project" }}' "${container_id}")"
  running_service="$(context_docker inspect --type container --format '{{ index .Config.Labels "com.docker.compose.service" }}' "${container_id}")"
  running_status="$(context_docker inspect --type container --format '{{.State.Status}}' "${container_id}")"
  running_health="$(context_docker inspect --type container --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${container_id}")"
  [[ "${running_image_id}" == "${configured_image_id}" && "${running_image_ref}" == "${configured_image}" ]] ||
    die "compose service ${service} is not running its exact configured image"
  [[ "${running_project}" == "${COMPOSE_PROJECT}" && "${running_service}" == "${service}" ]] ||
    die "compose service ${service} container labels differ from exact project authority"
  [[ "${running_status}" == "running" && "${running_health}" == "healthy" ]] ||
    die "compose service ${service} is ${running_status:-<missing>}/${running_health:-<missing>}, expected running/healthy"
  log "verified service ${service} container ${container_id} image ${configured_image_id} running healthy"
}

runtime_validate_core_url() {
  local core_url="$1" authority
  [[ -n "${core_url}" && ${#core_url} -le 4096 && ! "${core_url}" =~ [[:space:]] ]] ||
    die "CORE_URL must be one absolute HTTP or HTTPS URL"
  [[ "${core_url}" != *\?* && "${core_url}" != *\#* ]] ||
    die "CORE_URL must not contain a query or fragment"
  case "${core_url}" in
    http://*) authority="${core_url#http://}" ;;
    https://*) authority="${core_url#https://}" ;;
    *) die "CORE_URL must be one absolute HTTP or HTTPS URL" ;;
  esac
  authority="${authority%%/*}"
  [[ -n "${authority}" ]] || die "CORE_URL must include one network authority"
  [[ "${authority}" != *@* ]] || die "CORE_URL must not include credentials"
}

compose_require_public_health() (
  local container_id="$1" expected_commit="$2" core_url="$3"
  local body_file http_status health_commit
  [[ "${container_id}" =~ ^[0-9a-f]{12,64}$ ]] ||
    die "public health verification requires one exact core container"
  runtime_validate_build_commit "${expected_commit}"
  runtime_validate_core_url "${core_url}"
  command -v curl >/dev/null 2>&1 || die "curl is required for public core health verification"

  body_file="$(mktemp /tmp/omnidex-public-health.XXXXXX)" ||
    die "cannot allocate public core health response file"
  trap 'rm -f -- "${body_file}"' EXIT
  http_status="$(
    curl --disable --silent --show-error --fail \
      --retry 20 --retry-delay 1 --retry-all-errors --retry-max-time 30 \
      --max-time 10 --proto '=http,https' \
      --output "${body_file}" --write-out '%{http_code}' \
      "${core_url%/}/healthz"
  )" || die "configured public core health endpoint is unreachable or unhealthy"
  [[ "${http_status}" == "200" ]] ||
    die "configured public core health endpoint returned HTTP ${http_status:-<missing>}, expected 200"
  health_commit="$(
    context_docker exec -i "${container_id}" \
      /usr/local/bin/omnidex health --expect-commit "${expected_commit}" --stdin \
      < "${body_file}"
  )" || die "configured public core endpoint failed typed health verification"
  [[ "${health_commit}" == "${expected_commit}" ]] ||
    die "configured public core endpoint does not expose the expected running release"
  log "verified configured public core endpoint release commit ${health_commit}"
)
