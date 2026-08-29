#!/usr/bin/env bash

RELEASE_COMMIT_MANIFEST="RELEASE_COMMIT"

release_identity_validate_commit() {
  local commit="$1"
  [[ "${commit}" =~ ^[0123456789abcdef]{40}([0123456789abcdef]{24})?$ ]] ||
    die "release commit must be exactly 40 or 64 lowercase hexadecimal characters"
}

release_identity_read_manifest() {
  local root="$1" manifest commit
  manifest="${root}/${RELEASE_COMMIT_MANIFEST}"
  [[ -f "${manifest}" && ! -L "${manifest}" ]] ||
    die "release commit manifest must be a regular non-symlink file: ${manifest}"
  commit="$(sed -n '1p' "${manifest}")"
  release_identity_validate_commit "${commit}"
  cmp -s "${manifest}" <(printf '%s\n' "${commit}") ||
    die "release commit manifest must contain exactly one commit line"
  printf '%s\n' "${commit}"
}

release_identity_verify_json_binary() {
  local binary="$1" expected_commit="$2" output reported_commit
  output="$(OMNI_INVOKE_CWD= "${binary}" version --json)" ||
    die "release binary cannot report its embedded commit: ${binary}"
  reported_commit="$(printf '%s\n' "${output}" | sed -n 's/^[[:space:]]*"commit":[[:space:]]*"\([^"]*\)"[,]\{0,1\}[[:space:]]*$/\1/p')"
  [[ "${reported_commit}" == "${expected_commit}" ]] ||
    die "release binary reports commit ${reported_commit:-<missing>}, expected ${expected_commit}: ${binary}"
}

release_identity_verify_binaries() {
  local root="$1" expected_commit="$2" suffix="${3:-}"
  local core="${root}/bin/agent-core${suffix}"
  local cli="${root}/bin/agent-cli${suffix}"
  local omni="${root}/bin/omni${suffix}" output
  release_identity_validate_commit "${expected_commit}"
  for binary in "${core}" "${cli}" "${omni}"; do
    [[ -f "${binary}" && ! -L "${binary}" && -x "${binary}" ]] ||
      die "release binary must be a regular executable file: ${binary}"
  done
  output="$("${core}" release:verify-commit "${expected_commit}")" ||
    die "release core does not contain expected commit ${expected_commit}"
  [[ "${output}" == "${expected_commit}" ]] ||
    die "release core reports commit ${output:-<missing>}, expected ${expected_commit}"
  release_identity_verify_json_binary "${cli}" "${expected_commit}"
  release_identity_verify_json_binary "${omni}" "${expected_commit}"
}
