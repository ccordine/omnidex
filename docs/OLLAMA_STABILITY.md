# Ollama Stability Profile

## Observed Failure

Local `journalctl -u ollama` logs showed the model runner crashing under the
ROCm backend:

- `reason :GPU Hang`
- `systemd-coredump ... (ollama) ... dumped core`
- stack frames in `libhsa-runtime64.so.1`
- Omni then received Ollama `/api/chat` failures such as HTTP 500 and EOF.

The active local systemd drop-ins force ROCm:

- `OLLAMA_LLM_LIBRARY=rocm`
- `ROCR_VISIBLE_DEVICES=0`
- `OLLAMA_FLASH_ATTENTION=1`
- `HSA_OVERRIDE_GFX_VERSION=11.0.0`

## Source Findings

Ollama's troubleshooting docs say Linux systemd logs live at
`journalctl -u ollama --no-pager --follow --pager-end`, and that GPU crashes can
be worked around by forcing a specific LLM library with `OLLAMA_LLM_LIBRARY`.
They specifically list `cpu_avx2`, `cpu_avx`, and `cpu` as CPU library choices.

Ollama's hardware docs say AMD Radeon support on Linux uses ROCm, requires a
compatible ROCm driver, and supports `HSA_OVERRIDE_GFX_VERSION` for unsupported
or adjacent AMD GPU targets. Ollama's FAQ documents systemd environment
configuration, `keep_alive`, `OLLAMA_KEEP_ALIVE`, `OLLAMA_NUM_PARALLEL`, and
`OLLAMA_FLASH_ATTENTION`.

Sources:

- https://docs.ollama.com/troubleshooting
- https://docs.ollama.com/gpu
- https://docs.ollama.com/faq

## Applied Omni Changes

- Omni now supports default Ollama request controls:
  - `--ollama-keep-alive`, env `OMNI_OLLAMA_KEEP_ALIVE`, default `30s`
  - `--ollama-num-ctx`, env `OMNI_OLLAMA_NUM_CTX`, default `2048`
- Structured command failures now emit `structured_llm_backend_unstable` during
  transient Ollama runner failures.
- Final failed turns include a diagnosis such as
  `ollama_model_runner_crash_or_restart` when the error indicates runner
  instability.
- The retry backoff is longer so Ollama has time to restart the runner before
  the next structured request.

## Stable CPU Service Profile

Run this from the repo to install a CPU-first systemd drop-in:

```bash
./scripts/ollama-stable-cpu.sh
```

It writes `/etc/systemd/system/ollama.service.d/zz-omni-stable-cpu.conf` with:

```ini
[Service]
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
```

This prioritizes reliability over speed. The script removes the ROCm and Vulkan
Omni drop-ins before writing the CPU profile.

```bash
sudo systemctl daemon-reload
sudo systemctl restart ollama
```

## RX 7700S ROCm Profile

This machine exposes two ROCm GPU agents:

- `gfx1103` AMD Radeon 780M Graphics
- `gfx1102` AMD Radeon RX 7700S

The observed GPU hangs came from Ollama using the 780M path. To test the
dedicated RX 7700S path instead, run:

```bash
./scripts/ollama-rx7700s-rocm.sh
```

It removes the CPU fallback drop-in and writes
`/etc/systemd/system/ollama.service.d/zz-omni-rx7700s-rocm.conf` with:

```ini
[Service]
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
```

After restart, verify the logs show the RX 7700S/gfx1102 device rather than the
780M/gfx1103 device:

```bash
journalctl -u ollama --since '2 minutes ago' --no-pager | rg 'inference compute|using device|ROCm|GPU Hang'
```

If ROCm reports `total_vram="0 B"` or `inference compute id=cpu library=cpu`,
run the backend sanity script:

```bash
./scripts/ollama-backend-sanity.sh
```

Then try the Vulkan profile.

## Vulkan Profile

ROCm can see the RX 7700S through `rocminfo` while Ollama still falls back to
CPU. Vulkan is slower and experimental, but it is the next GPU path to test.
Install the Ollama Vulkan backend first:

```bash
sudo pacman -Syu ollama-vulkan vulkan-radeon vulkan-tools
```

Then apply the profile:

```bash
./scripts/ollama-vulkan.sh
```

It removes the CPU and ROCm Omni drop-ins and writes:

```ini
[Service]
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
```
