#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${script_dir}/ollama-profile-lib.sh"

dropin_dir="/etc/systemd/system/ollama.service.d"

if [[ "${EUID}" -ne 0 ]]; then
  exec sudo "$0" "$@"
fi

(($# == 0)) || {
  printf 'usage: sudo %s\n' "$0" >&2
  exit 1
}

ollama_require_one_omni_backend_profile "${dropin_dir}"
archive="${dropin_dir}/legacy-$(date -u +%Y%m%dT%H%M%SZ)"
ollama_archive_external_backend_dropins "${dropin_dir}" "${archive}"
ollama_require_no_external_backend_dropins "${dropin_dir}"

systemctl daemon-reload
systemctl restart ollama
systemctl status ollama --no-pager
printf 'legacy Ollama drop-ins preserved at %s\n' "${archive}"
