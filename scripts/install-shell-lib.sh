#!/usr/bin/env bash

remove_managed_block_file() {
  local file="$1"
  [[ -f "${file}" ]] || return 0
  local tmp
  tmp="$(mktemp)"
  awk -v start="${MANAGED_BLOCK_START}" -v end="${MANAGED_BLOCK_END}" '
    index($0, start) { skipping=1; next }
    index($0, end) { skipping=0; next }
    !skipping { print }
  ' "${file}" >"${tmp}"
  mv "${tmp}" "${file}"
}

append_managed_block_file() {
  local file="$1"
  remove_managed_block_file "${file}"
  cat >>"${file}" <<EOF

${MANAGED_BLOCK_START}
export OMNIDEX_DIR="${PREFIX}"
if [ -d "\$OMNIDEX_DIR/bin" ]; then
  case ":\$PATH:" in
    *":\$OMNIDEX_DIR/bin:"*) ;;
    *) export PATH="\$OMNIDEX_DIR/bin:\$PATH" ;;
  esac
fi
if [ -f "\$OMNIDEX_DIR/agent_aliases.sh" ]; then
  . "\$OMNIDEX_DIR/agent_aliases.sh"
fi
${MANAGED_BLOCK_END}
EOF
}

collect_shell_init_files() {
  local -a found=()
  local file
  for file in "${HOME}/.bashrc" "${HOME}/.bash_profile" "${HOME}/.profile" "${HOME}/.zshrc"; do
    [[ ! -f "${file}" ]] || found+=("${file}")
  done
  if ((${#found[@]} == 0)); then
    local fallback="${HOME}/.bashrc"
    [[ "$(basename "${SHELL:-bash}")" != "zsh" ]] || fallback="${HOME}/.zshrc"
    touch "${fallback}"
    found+=("${fallback}")
  fi
  printf '%s\n' "${found[@]}"
}

integrate_shell_init() {
  local file count=0
  while IFS= read -r file; do
    [[ -n "${file}" ]] || continue
    append_managed_block_file "${file}"
    log "updated shell init: ${file}"
    count=$((count + 1))
  done < <(collect_shell_init_files)
  ((count > 0)) || die "no shell init files were updated"
}
