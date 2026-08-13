#!/usr/bin/env bash

managed_checkout_require_source() {
  local repository="$1"
  command -v git >/dev/null 2>&1 || die "git is required for the authoritative install/update checkout"
  [[ -d "${repository}/.git" && ! -L "${repository}/.git" ]] ||
    die "source must be a complete Git checkout with a real .git directory: ${repository}"
  git -C "${repository}" diff --quiet --ignore-submodules -- ||
    die "source checkout has unstaged tracked changes"
  git -C "${repository}" diff --cached --quiet --ignore-submodules -- ||
    die "source checkout has staged tracked changes"
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

managed_checkout_preserve_env() {
  local current="$1" stage="$2"
  if [[ -e "${current}/.env" ]]; then
    [[ -f "${current}/.env" && ! -L "${current}/.env" ]] ||
      die "existing .env must be a regular file"
    cp -p "${current}/.env" "${stage}/.env"
  else
    [[ -f "${stage}/default.env" ]] || die "default.env is missing from staged checkout"
    cp -p "${stage}/default.env" "${stage}/.env"
  fi
}

managed_checkout_validate_env() {
  local stage="$1"
  local core="${stage}/bin/agent-core"
  local environment="${stage}/.env"
  [[ -x "${core}" ]] || die "staged agent-core is not executable"
  [[ -f "${environment}" && ! -L "${environment}" ]] ||
    die "staged .env must be a regular file"
  "${core}" config:validate-file "${environment}" >/dev/null ||
    die "staged .env is incompatible with this Omnidex build"
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

managed_checkout_publish() {
  local stage="$1" target="$2" parent base backup=""
  parent="$(dirname "${target}")"
  base="$(basename "${target}")"
  [[ "$(dirname "${stage}")" == "${parent}" ]] || die "checkout stage must share the install parent"
  if [[ -e "${target}" || -L "${target}" ]]; then
    [[ -d "${target}" && ! -L "${target}" ]] || die "install target must be a real directory"
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
