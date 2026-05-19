#!/usr/bin/env bash
set -euo pipefail

echo "== packages =="
has_ollama_vulkan=0
if command -v pacman >/dev/null 2>&1; then
  pacman -Qs '^ollama' || true
  if pacman -Qi ollama-vulkan >/dev/null 2>&1; then
    has_ollama_vulkan=1
  fi
  pacman -Qi ollama ollama-rocm ollama-vulkan 2>/dev/null | sed -n '/^Name/{N;N;N;p;}' || true
  pacman -Qi vulkan-radeon vulkan-tools 2>/dev/null | sed -n '/^Name/{N;N;N;p;}' || true
else
  echo "pacman not found"
fi
if [[ "${has_ollama_vulkan}" -eq 0 ]]; then
  echo "missing: ollama-vulkan (required for OLLAMA_VULKAN=1)"
fi

echo
echo "== ollama backend libraries =="
find /usr/lib/ollama -maxdepth 2 -type f \( -iname '*vulkan*' -o -iname '*vk*' -o -iname '*hip*' -o -iname '*rocm*' \) 2>/dev/null | sort || true

echo
echo "== service =="
systemctl show ollama -p ActiveState -p MainPID -p User -p Group -p SupplementaryGroups -p Environment --no-pager || true

echo
echo "== drm devices =="
ls -l /dev/kfd /dev/dri 2>/dev/null || true
for n in /sys/class/drm/renderD*/device; do
  [[ -e "${n}" ]] || continue
  echo "${n} -> $(readlink -f "${n}")"
  printf "vendor="; cat "${n}/vendor" 2>/dev/null || true
  printf "device="; cat "${n}/device" 2>/dev/null || true
done

echo
echo "== rocm visible to current user =="
if command -v rocminfo >/dev/null 2>&1; then
  rocminfo | rg 'Name:|Marketing Name|gfx' || true
else
  echo "rocminfo not found"
fi

echo
echo "== rocm rx7700s only as ollama user =="
if command -v sudo >/dev/null 2>&1 && command -v rocminfo >/dev/null 2>&1; then
  sudo -n -u ollama ROCR_VISIBLE_DEVICES=1 rocminfo 2>/dev/null | rg 'Name:|Marketing Name|gfx' || echo "sudo without password unavailable; run: sudo -u ollama ROCR_VISIBLE_DEVICES=1 rocminfo | rg 'gfx|Marketing Name'"
else
  echo "sudo or rocminfo not found"
fi

echo
echo "== vulkan summary =="
if command -v vulkaninfo >/dev/null 2>&1; then
  vulkaninfo --summary 2>/dev/null | rg 'GPU id|deviceName|driverName|apiVersion' || true
else
  echo "vulkaninfo not found"
fi

echo
echo "== recent ollama backend logs =="
journalctl -u ollama --since '20 minutes ago' --no-pager | rg 'server config|inference compute|total_vram|model weights|load_backend|failed to initialize|ROCm|Vulkan|gfx|GPU Hang|POST' || true
