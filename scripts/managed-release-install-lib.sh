#!/usr/bin/env bash

MANAGED_RELEASE_LIBRARY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${MANAGED_RELEASE_LIBRARY_DIR}/release-binary-identity-lib.sh"

MANAGED_RELEASE_STAGE=""
MANAGED_RELEASE_TARGET=""

managed_release_cleanup_stage() {
  [[ -z "${MANAGED_RELEASE_STAGE}" ]] || rm -rf -- "${MANAGED_RELEASE_STAGE}"
  MANAGED_RELEASE_STAGE=""
}

managed_release_root() {
  local script_dir="$1" root
  root="$(cd "${script_dir}" && pwd -P)"
  [[ -x "${root}/bin/omni" && -x "${root}/bin/agent-cli" && -x "${root}/bin/agent-core" ]] ||
    die "release archive is missing one or more required binaries"
  [[ -f "${root}/default.env" && ! -L "${root}/default.env" ]] ||
    die "release archive is missing default.env"
  release_identity_read_manifest "${root}" >/dev/null
  [[ ! -e "${root}/.env" && ! -L "${root}/.env" ]] ||
    die "release archive must not contain an active .env"
  printf '%s\n' "${root}"
}

managed_release_publish() {
  local source="$1" requested_target="$2" explicit_env="$3"
  local parent base target backup="" existing_env="" env_source="" release_commit
  [[ -d "${source}" && ! -L "${source}" ]] || die "release source must be a real directory"
  source="$(cd "${source}" && pwd -P)"

  parent="$(dirname "${requested_target}")"
  base="$(basename "${requested_target}")"
  [[ "${base}" != "." && "${base}" != ".." ]] || die "install target name is unsafe"
  mkdir -p "${parent}"
  [[ -d "${parent}" && ! -L "${parent}" ]] || die "install parent must be a real directory: ${parent}"
  parent="$(cd "${parent}" && pwd -P)"
  target="${parent}/${base}"
  [[ "${target}" != "${source}" ]] || die "install target must differ from the release source"
  case "${target}/" in
    "${source}/"*) die "install target must not be inside the release source" ;;
  esac
  case "${source}/" in
    "${target}/"*) die "install target must not contain the release source" ;;
  esac

  if [[ -e "${target}" || -L "${target}" ]]; then
    [[ -d "${target}" && ! -L "${target}" ]] || die "install target must be a real directory"
    if [[ -n "$(find "${target}" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
      [[ -x "${target}/bin/omni" && -x "${target}/bin/agent-cli" && -x "${target}/bin/agent-core" ]] ||
        die "existing non-empty target is not a managed Omnidex release"
      [[ -f "${target}/.env" && ! -L "${target}/.env" ]] ||
        die "existing managed .env must be a regular file"
      [[ -z "${explicit_env}" ]] ||
        die "--env-file cannot replace an existing managed .env"
      existing_env="${target}/.env"
    fi
  fi

  if [[ -n "${existing_env}" ]]; then
    env_source="${existing_env}"
  else
    [[ -n "${explicit_env}" ]] ||
      die "fresh release installation requires --env-file PATH; default.env is a template only"
    [[ -f "${explicit_env}" && ! -L "${explicit_env}" ]] ||
      die "--env-file must name a regular non-symlink file"
    env_source="$(cd "$(dirname "${explicit_env}")" && pwd -P)/$(basename "${explicit_env}")"
    case "${env_source}" in
      "${source}/"*) die "--env-file must be outside the extracted release directory" ;;
    esac
  fi

  MANAGED_RELEASE_STAGE="$(mktemp -d "${parent}/.${base}.release-install.XXXXXX")"
  cp -a "${source}/." "${MANAGED_RELEASE_STAGE}/"
  [[ ! -e "${MANAGED_RELEASE_STAGE}/.env" && ! -L "${MANAGED_RELEASE_STAGE}/.env" ]] ||
    die "release payload unexpectedly contains an active .env"
  cp -p "${env_source}" "${MANAGED_RELEASE_STAGE}/.env"
  release_commit="$(release_identity_read_manifest "${MANAGED_RELEASE_STAGE}")"
  release_identity_verify_binaries "${MANAGED_RELEASE_STAGE}" "${release_commit}"
  [[ -x "${MANAGED_RELEASE_STAGE}/bin/agent-core" ]] ||
    die "staged release agent-core is not executable"
  [[ -x "${MANAGED_RELEASE_STAGE}/bin/agent-cli" ]] ||
    die "staged release agent-cli is not executable"
  "${MANAGED_RELEASE_STAGE}/bin/agent-core" config:validate-file "${MANAGED_RELEASE_STAGE}/.env" >/dev/null ||
    die "staged .env is incompatible with this Omnidex release"
  "${MANAGED_RELEASE_STAGE}/bin/agent-cli" config:validate-file "${MANAGED_RELEASE_STAGE}/.env" >/dev/null ||
    die "staged .env does not provide valid managed CLI authority"

  if [[ -e "${target}" || -L "${target}" ]]; then
    backup="$(mktemp -d "${parent}/.${base}.previous.XXXXXX")"
    rmdir "${backup}"
    if ! mv "${target}" "${backup}"; then
      die "failed to stage the prior managed install for publication"
    fi
  fi
  if ! mv "${MANAGED_RELEASE_STAGE}" "${target}"; then
    if [[ -n "${backup}" ]]; then
      mv "${backup}" "${target}" ||
        die "publication failed and prior install rollback failed: ${backup}"
    fi
    die "failed to publish staged release"
  fi
  MANAGED_RELEASE_STAGE=""
  if [[ -n "${backup}" ]]; then
    rm -rf -- "${backup}"
  fi
  MANAGED_RELEASE_TARGET="${target}"
}
