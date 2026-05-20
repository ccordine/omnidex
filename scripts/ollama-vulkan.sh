#!/usr/bin/env bash
set -euo pipefail

dropin_dir="/etc/systemd/system/ollama.service.d"
dropin_file="${dropin_dir}/zz-omni-vulkan.conf"
cpu_dropin="${dropin_dir}/zz-omni-stable-cpu.conf"
rocm_dropin="${dropin_dir}/zz-omni-rx7700s-rocm.conf"

if [[ "${EUID}" -ne 0 ]]; then
  exec sudo "$0" "$@"
fi

if command -v pacman >/dev/null 2>&1 && ! pacman -Qi ollama-vulkan >/dev/null 2>&1; then
  echo "ollama-vulkan is not installed. Install it first: sudo pacman -Syu ollama-vulkan vulkan-radeon vulkan-tools" >&2
  exit 1
fi

if ! find /usr/lib/ollama -maxdepth 2 -type f \( -iname '*vulkan*' -o -iname '*vk*' \) | grep -q .; then
  echo "No Vulkan backend library found under /usr/lib/ollama. Install the Ollama Vulkan backend package first." >&2
  exit 1
fi

install -d -m 0755 "${dropin_dir}"
rm -f "${cpu_dropin}" "${rocm_dropin}"
cat > "${dropin_file}" <<'EOF'
[Service]
# Omni Vulkan experiment profile.
# Vulkan is experimental in Ollama, but can be a useful fallback when ROCm
# detects the hardware through rocminfo but Ollama still reports total_vram=0.
Environment="OLLAMA_LLM_LIBRARY="
Environment="OLLAMA_VULKAN=1"
Environment="GGML_VK_VISIBLE_DEVICES=1"
Environment="ROCR_VISIBLE_DEVICES="
Environment="HIP_VISIBLE_DEVICES="
Environment="GPU_DEVICE_ORDINAL="
Environment="CUDA_VISIBLE_DEVICES="
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
