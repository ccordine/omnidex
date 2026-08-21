package api

import (
	"strings"
	"testing"
)

func TestBrowserContextRelevanceClientRemainsOneProviderBridge(t *testing.T) {
	controller := readFrontendSource(t, "web/src/controllers/browser_inference_controller.ts")
	runtime := readFrontendSource(t, "web/src/lib/browser_context_relevance_runtime.ts")
	protocol := readFrontendSource(t, "web/src/lib/browser_context_relevance_protocol.ts")
	worker := readFrontendSource(t, "web/src/workers/browser_inference_worker.ts")

	for path, source := range map[string]string{
		"controller": controller, "runtime": runtime, "protocol": protocol, "worker": worker,
	} {
		if lines := strings.Count(source, "\n") + 1; lines > 300 {
			t.Errorf("browser inference %s has %d lines", path, lines)
		}
		for _, forbidden := range []string{"innerHTML", "insertAdjacentHTML", "tools:", "tool_choice"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("browser inference %s contains forbidden client authority %q", path, forbidden)
			}
		}
	}
	for _, required := range []string{
		"this.element !== document.body",
		"runtime.stop()",
		"new BrowserContextRelevanceRuntime()",
	} {
		if !strings.Contains(controller, required) {
			t.Errorf("browser inference controller lacks %q", required)
		}
	}
	if strings.Index(runtime, "if (!config.enabled)") > strings.Index(runtime, `import("@mlc-ai/web-llm")`) {
		t.Fatal("server execution imports WebLLM before the deterministic disabled fast path")
	}
	for _, required := range []string{
		`import("@mlc-ai/web-llm")`,
		`cacheBackend: "opfs"`,
		"await this.engine.resetChat(false)",
		"requireBrowserContextJob",
		"browserContextProviderResult",
	} {
		if !strings.Contains(runtime, required) {
			t.Errorf("browser inference runtime lacks %q", required)
		}
	}
}
