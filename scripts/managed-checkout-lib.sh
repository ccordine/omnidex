#!/usr/bin/env bash

managed_checkout_require_source() {
  local repository="$1" status
  command -v git >/dev/null 2>&1 || die "git is required for the authoritative install/update checkout"
  [[ -d "${repository}/.git" && ! -L "${repository}/.git" ]] ||
    die "source must be a complete Git checkout with a real .git directory: ${repository}"
  status="$(git -C "${repository}" status --porcelain=v1 --untracked-files=normal --ignore-submodules=none)" ||
    die "cannot inspect source checkout state: ${repository}"
  [[ -z "${status}" ]] || die "source checkout is dirty, including tracked, untracked, or submodule changes"
}





managed_checkout_require_replaceable_target() {
  local repository="$1" path unexpected=""
  if [[ ! -d "${repository}/.git" ]]; then
    [[ -z "$(find "${repository}" -mindepth 1 -maxdepth 1 -print -quit)" ]] ||
      die "existing non-empty target is not a managed Omnidex checkout: ${repository}"
    return
  fi
  managed_checkout_require_source "${repository}"
  while IFS= read -r -d '' path; do
    case "${path}" in
      .env|bin/*|internal/api/web/node_modules/*|internal/api/web/dist/*)
        ;;
      *)
        unexpected="${path}"
        break
        ;;
    esac
  done < <(git -C "${repository}" ls-files -z --others --ignored --exclude-standard)
  [[ -z "${unexpected}" ]] ||
    die "managed checkout contains unmanaged files that publication would remove: ${unexpected}"
}

managed_checkout_branch() {
  local repository="$1" requested="$2" branch
  branch="${requested}"
  if [[ -z "${branch}" ]]; then
    branch="$(git -C "${repository}" symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  fi
  [[ -n "${branch}" && "${branch}" != -* && "${branch}" != *$'\n'* && "${branch}" != *$'\r'* ]] ||
    die "source checkout must be on an explicit branch"
  git -C "${repository}" check-ref-format --branch "${branch}" >/dev/null 2>&1 ||
    die "invalid update branch: ${branch}"
  printf '%s\n' "${branch}"
}

managed_checkout_origin() {
  local repository="$1" origin
  origin="$(git -C "${repository}" remote get-url origin 2>/dev/null || true)"
  [[ -n "${origin}" && "${origin}" != *$'\n'* && "${origin}" != *$'\r'* ]] ||
    die "source checkout must have one usable origin remote"
  printf '%s\n' "${origin}"
}

managed_checkout_new_stage() {
  local target="$1" purpose="$2" parent base stage
  parent="$(dirname "${target}")"
  base="$(basename "${target}")"
  mkdir -p "${parent}"
  [[ -d "${parent}" && ! -L "${parent}" ]] || die "install parent must be a real directory: ${parent}"
  parent="$(cd "${parent}" && pwd -P)"
  stage="$(mktemp -d "${parent}/.${base}.${purpose}.XXXXXX")"
  [[ -d "${stage}" && ! -L "${stage}" ]] || die "unable to create checkout stage"
  printf '%s\n' "${stage}"
}

managed_checkout_clone_exact() {
  local source="$1" stage="$2" branch="$3" origin="$4" expected_head actual_head
  expected_head="$(git -C "${source}" rev-parse "${branch}")"
  git clone --quiet --no-hardlinks --no-local --branch "${branch}" --single-branch "${source}" "${stage}"
  actual_head="$(git -C "${stage}" rev-parse HEAD)"
  [[ "${actual_head}" == "${expected_head}" ]] || die "staged checkout does not match source HEAD"
  git -C "${stage}" remote set-url origin "${origin}"
  [[ -z "$(git -C "${stage}" ls-files --deleted)" ]] || die "staged checkout has tracked deletions"
  [[ -z "$(git -C "${stage}" status --porcelain=1 --untracked-files=all)" ]] ||
    die "staged checkout is not clean"
}

managed_checkout_fast_forward() {
  local stage="$1" branch="$2"
  git -C "${stage}" fetch --prune origin "${branch}"
  git -C "${stage}" merge --ff-only "origin/${branch}"
  [[ -z "$(git -C "${stage}" status --porcelain=1 --untracked-files=all)" ]] ||
    die "updated staged checkout is not clean"
}

managed_checkout_stage_env() {
  local current="$1" stage="$2" explicit_env="$3" source=""
  if [[ -e "${current}/.env" || -L "${current}/.env" ]]; then
    [[ -f "${current}/.env" && ! -L "${current}/.env" ]] ||
      die "existing managed .env must be a regular file"
    [[ -z "${explicit_env}" ]] ||
      die "--env-file cannot replace an existing managed .env"
    source="${current}/.env"
  else
    [[ -n "${explicit_env}" ]] ||
      die "fresh checkout installation requires --env-file PATH; .env.example is a template only"
    [[ -f "${explicit_env}" && ! -L "${explicit_env}" ]] ||
      die "--env-file must name a regular non-symlink file"
    source="${explicit_env}"
  fi
  [[ ! -e "${stage}/.env" && ! -L "${stage}/.env" ]] ||
    die "staged checkout unexpectedly contains an active .env"
  cp -p "${source}" "${stage}/.env"
}

managed_checkout_validate_stage() {
  local stage="$1"
  local server="${stage}/bin/omnidex"
  local cli="${stage}/bin/omni"
  local environment="${stage}/.env"
  local path retired
  [[ -d "${stage}/bin" && ! -L "${stage}/bin" ]] ||
    die "staged bin must be one real directory"
  [[ -f "${server}" && ! -L "${server}" && -x "${server}" ]] ||
    die "staged bin/omnidex must be one regular executable server binary"
  [[ -f "${cli}" && ! -L "${cli}" && -x "${cli}" ]] ||
    die "staged bin/omni must be one regular executable CLI binary"
  for retired in agent-core agent-cli acli; do
    [[ ! -e "${stage}/bin/${retired}" && ! -L "${stage}/bin/${retired}" ]] ||
      die "staged install contains retired binary or alias: bin/${retired}"
  done
  [[ -f "${environment}" && ! -L "${environment}" ]] ||
    die "staged .env must be a regular file"
  while IFS= read -r -d '' path; do
    case "${path}" in
      "${server}"|"${cli}") ;;
      *) die "staged bin contains an unsupported install artifact: bin/$(basename "${path}")" ;;
    esac
  done < <(find "${stage}/bin" -mindepth 1 -maxdepth 1 -print0)
}

managed_checkout_env_value() {
  local file="$1" key="$2"
  [[ -f "${file}" && ! -L "${file}" ]] || die "managed .env must be a regular file"
  awk -v wanted="${key}" '
    BEGIN { count=0; value="" }
    index($0, wanted "=") == 1 {
      count++
      value=substr($0, length(wanted) + 2)
    }
    END {
      if (count > 1) exit 3
      if (count == 1) print value
    }
  ' "${file}" || die "managed .env defines ${key} more than once"
}

managed_checkout_require_env_key() {
  local file="$1" key="$2"
  [[ -f "${file}" && ! -L "${file}" ]] || die "managed .env must be a regular file"
  awk -v wanted="${key}" '
    BEGIN { count=0 }
    index($0, wanted "=") == 1 { count++ }
    END { exit count == 1 ? 0 : 1 }
  ' "${file}" || die "managed .env must define ${key} exactly once"
}

managed_checkout_publish() {
  local stage="$1" target="$2" parent base backup=""
  parent="$(dirname "${target}")"
  base="$(basename "${target}")"
  [[ "$(dirname "${stage}")" == "${parent}" ]] || die "checkout stage must share the install parent"
  if [[ -e "${target}" || -L "${target}" ]]; then
    [[ -d "${target}" && ! -L "${target}" ]] || die "install target must be a real directory"
    managed_checkout_require_replaceable_target "${target}"
    backup="$(mktemp -d "${parent}/.${base}.previous.XXXXXX")"
    rmdir "${backup}"
    mv "${target}" "${backup}"
  fi
  if ! mv "${stage}" "${target}"; then
    if [[ -n "${backup}" ]]; then
      mv "${backup}" "${target}" || die "publication failed and prior install rollback failed: ${backup}"
    fi
    die "failed to publish staged checkout"
  fi
  if [[ -n "${backup}" ]]; then
    rm -rf -- "${backup}"
  fi
}

managed_checkout_build_binaries() (
  local repository="$1" build_dir
  mkdir -p "${repository}/bin"
  build_dir="$(mktemp -d "${repository}/bin/.omnidex-build.XXXXXX")"
  trap 'rm -f "${build_dir}/omnidex" "${build_dir}/omni"; rmdir "${build_dir}"' EXIT
  "${repository}/scripts/build-core.sh" --package ./cmd/omnidex --output "${build_dir}/omnidex"
  "${repository}/scripts/build-core.sh" --package ./cmd/omni --output "${build_dir}/omni"
  mv -f "${build_dir}/omnidex" "${repository}/bin/omnidex"
  mv -f "${build_dir}/omni" "${repository}/bin/omni"
)
