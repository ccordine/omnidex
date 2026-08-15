#!/usr/bin/env bash

ollama_backend_environment_pattern='^[[:space:]]*Environment="(OLLAMA_LLM_LIBRARY|OLLAMA_VULKAN|ROCR_VISIBLE_DEVICES|HIP_VISIBLE_DEVICES|GPU_DEVICE_ORDINAL|CUDA_VISIBLE_DEVICES|GGML_VK_VISIBLE_DEVICES|HSA_OVERRIDE_GFX_VERSION|OLLAMA_NUM_PARALLEL|OLLAMA_MAX_LOADED_MODELS|OLLAMA_KEEP_ALIVE|OLLAMA_FLASH_ATTENTION)='

ollama_is_managed_backend_dropin() {
  case "$1" in
    zz-omni-stable-cpu.conf|zz-omni-rx7700s-rocm.conf|zz-omni-vulkan.conf|zz-odn-vulkan.conf)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

ollama_require_no_external_backend_dropins() {
  local directory="$1" file name
  [[ -d "${directory}" ]] || return 0
  for file in "${directory}"/*.conf; do
    [[ -f "${file}" ]] || continue
    name="$(basename "${file}")"
    ollama_is_managed_backend_dropin "${name}" && continue
    if grep -Eq "${ollama_backend_environment_pattern}" "${file}"; then
      printf 'conflicting Ollama backend drop-in must be archived or removed: %s\n' "${file}" >&2
      return 1
    fi
  done
}

ollama_require_one_omni_backend_profile() {
  local directory="$1" path count=0
  for path in \
    "${directory}/zz-omni-stable-cpu.conf" \
    "${directory}/zz-omni-rx7700s-rocm.conf" \
    "${directory}/zz-omni-vulkan.conf"; do
    [[ ! -e "${path}" && ! -L "${path}" ]] || {
      [[ -f "${path}" && ! -L "${path}" ]] || {
        printf 'managed Ollama backend profile must be a regular non-symlink file: %s\n' "${path}" >&2
        return 1
      }
      count=$((count + 1))
    }
  done
  [[ "${count}" -eq 1 ]] || {
    printf 'expected exactly one managed Ollama backend profile, found %s\n' "${count}" >&2
    return 1
  }
}

ollama_archive_external_backend_dropins() {
  local directory="$1" archive="$2" file name temporary
  [[ ! -e "${archive}" && ! -L "${archive}" ]] || {
    printf 'Ollama drop-in archive already exists: %s\n' "${archive}" >&2
    return 1
  }
  install -d -m 0755 "${archive}"
  for file in "${directory}"/*.conf; do
    [[ -f "${file}" && ! -L "${file}" ]] || continue
    name="$(basename "${file}")"
    ollama_is_managed_backend_dropin "${name}" && continue
    grep -Eq "${ollama_backend_environment_pattern}" "${file}" || continue
    cp -p -- "${file}" "${archive}/${name}"
    temporary="$(mktemp "${directory}/.omni-dropin.XXXXXX")"
    awk '
      $0 ~ /^[[:space:]]*Environment="(OLLAMA_LLM_LIBRARY|OLLAMA_VULKAN|ROCR_VISIBLE_DEVICES|HIP_VISIBLE_DEVICES|GPU_DEVICE_ORDINAL|CUDA_VISIBLE_DEVICES|GGML_VK_VISIBLE_DEVICES|HSA_OVERRIDE_GFX_VERSION|OLLAMA_NUM_PARALLEL|OLLAMA_MAX_LOADED_MODELS|OLLAMA_KEEP_ALIVE|OLLAMA_FLASH_ATTENTION)=/ { next }
      { print }
    ' "${file}" >"${temporary}"
    chmod --reference="${file}" "${temporary}"
    if awk '
      /^[[:space:]]*$/ { next }
      /^[[:space:]]*#/ { next }
      /^[[:space:]]*\[[^]]+\][[:space:]]*$/ { next }
      { found=1 }
      END { exit found ? 0 : 1 }
    ' "${temporary}"; then
      mv -f -- "${temporary}" "${file}"
    else
      rm -f -- "${temporary}" "${file}"
    fi
    printf 'archived and retired external Ollama backend settings: %s\n' "${file}"
  done
}
