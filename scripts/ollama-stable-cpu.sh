#!/usr/bin/env bash
set -euo pipefail

dropin_dir="/etc/systemd/system/ollama.service.d"
dropin_file="${dropin_dir}/zz-omni-stable-cpu.conf"
rocm_dropin="${dropin_dir}/zz-omni-rx7700s-rocm.conf"
vulkan_dropin="${dropin_dir}/zz-omni-vulkan.conf"

if [[ "${EUID}" -ne 0 ]]; then
  exec sudo "$0" "$@"
fi

install -d -m 0755 "${dropin_dir}"
rm -f "${rocm_dropin}" "${vulkan_dropin}"
cat > "${dropin_file}" <<'EOF'
[Service]
# Omni stability profile.
# The local logs showed ROCm/HSA GPU hangs on the Radeon 780M path. Ollama's
# troubleshooting docs recommend forcing a specific LLM library when GPU
# crashes occur; cpu_avx2 is the fastest CPU fallback when available.
Environment="OLLAMA_LLM_LIBRARY=cpu_avx2"
Environment="ROCR_VISIBLE_DEVICES=-1"
Environment="HIP_VISIBLE_DEVICES=-1"
Environment="CUDA_VISIBLE_DEVICES=-1"
Environment="GPU_DEVICE_ORDINAL=-1"
Environment="GGML_VK_VISIBLE_DEVICES=-1"
Environment="HSA_OVERRIDE_GFX_VERSION="
Environment="OLLAMA_NUM_PARALLEL=1"
Environment="OLLAMA_MAX_LOADED_MODELS=1"
Environment="OLLAMA_KEEP_ALIVE=30s"
Environment="OLLAMA_FLASH_ATTENTION=0"
EOF

systemctl daemon-reload
systemctl restart ollama
systemctl status ollama --no-pager
