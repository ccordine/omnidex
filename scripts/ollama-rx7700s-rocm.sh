#!/usr/bin/env bash
set -euo pipefail

dropin_dir="/etc/systemd/system/ollama.service.d"
dropin_file="${dropin_dir}/zz-odn-rx7700s-rocm.conf"
cpu_dropin="${dropin_dir}/zz-odn-stable-cpu.conf"
vulkan_dropin="${dropin_dir}/zz-odn-vulkan.conf"

if [[ "${EUID}" -ne 0 ]]; then
  exec sudo "$0" "$@"
fi

install -d -m 0755 "${dropin_dir}"
rm -f "${cpu_dropin}" "${vulkan_dropin}"
cat > "${dropin_file}" <<'EOF'
[Service]
# ODN RX 7700S ROCm profile.
# rocminfo reports:
#   GPU 0: gfx1103 AMD Radeon 780M Graphics
#   GPU 1: gfx1102 AMD Radeon RX 7700S
# The previous GPU hangs came from exposing device 0. Pin Ollama to device 1.
Environment="OLLAMA_LLM_LIBRARY=rocm"
Environment="ROCR_VISIBLE_DEVICES=1"
Environment="HIP_VISIBLE_DEVICES="
Environment="GPU_DEVICE_ORDINAL="
Environment="CUDA_VISIBLE_DEVICES="
Environment="GGML_VK_VISIBLE_DEVICES="
Environment="HSA_OVERRIDE_GFX_VERSION="
Environment="OLLAMA_NUM_PARALLEL=1"
Environment="OLLAMA_MAX_LOADED_MODELS=1"
Environment="OLLAMA_KEEP_ALIVE=30s"
Environment="OLLAMA_FLASH_ATTENTION=0"
SupplementaryGroups=render video
EOF

systemctl daemon-reload
systemctl restart ollama
systemctl status ollama --no-pager
