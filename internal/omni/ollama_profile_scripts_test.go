package omni

import (
	"strings"
	"testing"
)

func TestOllamaBackendProfilesRetireLegacyVulkanOverride(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, scriptName := range []string{
		"scripts/ollama-stable-cpu.sh",
		"scripts/ollama-rx7700s-rocm.sh",
		"scripts/ollama-vulkan.sh",
	} {
		body := readRepoScript(t, root, scriptName)
		for _, required := range []string{
			"legacy_vulkan_dropin=\"${dropin_dir}/zz-odn-vulkan.conf\"",
			"\"${legacy_vulkan_dropin}\"",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s leaves legacy Vulkan override active: missing %q", scriptName, required)
			}
		}
	}

	for _, scriptName := range []string{
		"scripts/ollama-stable-cpu.sh",
		"scripts/ollama-rx7700s-rocm.sh",
	} {
		if !strings.Contains(readRepoScript(t, root, scriptName), `Environment="OLLAMA_VULKAN="`) {
			t.Fatalf("%s does not explicitly clear Vulkan selection", scriptName)
		}
	}
}
