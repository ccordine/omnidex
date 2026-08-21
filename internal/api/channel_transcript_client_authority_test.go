package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChannelTranscriptHasNoBrowserRendererOrLocalMessageState(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("web/src/lib/chat_render.ts"); err == nil {
		t.Fatal("browser chat component renderer still exists")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	checks := map[string][]string{
		"web/src/lib/channel_api.ts": {
			"payload.messages", "ChannelMessagePage", "requireChannelMessage(item",
		},
		"web/src/lib/chat_channel_coordinator.ts": {
			"ChatMessage[]", ".messages.map(", "replaceMessages(", "page.messages",
		},
		"web/src/controllers/chat_view_controller.ts": {
			"renderChatMessages", "messages: ChatMessage[]", "private messages", "this.messages =", "this.messages.push", "renderMessages()",
		},
	}
	for path, forbidden := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("browser transcript authority %q remains in %s", token, path)
			}
		}
	}
}

func TestChannelTranscriptBrowserConsumesServerBundleThroughPageRecyclr(t *testing.T) {
	t.Parallel()
	coordinator := readFrontendSource(t, "web/src/lib/chat_channel_coordinator.ts")
	for _, required := range []string{"fetchChannelTranscript(", "renderTranscriptBundle(page.html.bundle"} {
		if !strings.Contains(coordinator, required) {
			t.Errorf("channel coordinator lacks %q", required)
		}
	}
	view := readFrontendSource(t, "web/src/controllers/chat_view_controller.ts")
	for _, required := range []string{"recyclrController.renderBundle(bundle)", "aria-busy"} {
		if !strings.Contains(view, required) {
			t.Errorf("chat view lacks %q", required)
		}
	}
}

func TestChatOperationalComponentsHaveNoBrowserMarkupAuthority(t *testing.T) {
	t.Parallel()
	for _, removed := range []string{
		"web/src/lib/chat_render.ts",
		"web/src/lib/chat_progress_view.ts",
		"web/src/lib/render.ts",
	} {
		if _, err := os.Stat(removed); err == nil {
			t.Errorf("removed browser component remains: %s", removed)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	checks := map[string][]string{
		"web/src/controllers/chat_view_controller.ts": {
			"events: TimelineEvent[]", "eventIndex", "contextIndex", "renderTimeline(",
			"renderEventModal", "renderContextModal", "renderChatProgress",
		},
		"web/src/lib/chat_jobs_coordinator.ts": {
			"renderJobsPanel", "renderStep(", "renderContext(", "renderStepSummary",
			"escapeHTML", "statusPillClass", "formatDateTime",
		},
		"web/src/lib/chat_memory_coordinator.ts": {
			"renderList(", "renderCandidates(", "emptyState", "escapeHTML", "statusPillClass",
		},
		"web/src/lib/chat_channel_coordinator.ts": {
			"new Option(", "channels: UserChannel[]", "renderOptions(", "replaceChildren(", "createElement(",
		},
		"web/src/lib/chat_system_coordinator.ts": {
			"renderHostBridgeStatus", "renderMetricsDashboard", "errorPanel(", "innerHTML", "insertAdjacentHTML",
		},
	}
	for path, forbidden := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("browser component authority %q remains in %s", token, path)
			}
		}
	}
}

func TestAllChatProductionTypeScriptHasNoComponentConstructionAPI(t *testing.T) {
	t.Parallel()
	patterns := []string{"web/src/lib/chat_*.ts", "web/src/controllers/chat_*.ts"}
	forbidden := []string{
		".innerHTML", "insertAdjacentHTML(", "document.createElement(", "new Option(",
		"replaceChildren(", "renderRecyclrBundle(", "buildRecyclrBundle(", "`<",
	}
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if strings.HasSuffix(path, ".test.ts") {
				continue
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range forbidden {
				if strings.Contains(string(raw), token) {
					t.Errorf("chat production TypeScript constructs browser components with %q in %s", token, path)
				}
			}
		}
	}
}

func TestChatSystemPanelsConsumeServerComponents(t *testing.T) {
	t.Parallel()
	source := readFrontendSource(t, "web/src/lib/chat_system_coordinator.ts")
	for _, required := range []string{
		`fetch("/v1/host/status")`,
		`fetch("/v1/status/research")`,
		`fetch("/v1/ui/chat/metrics")`,
		"requireServerComponentBundle(",
		"renderComponentBundle(",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("chat system coordinator lacks server component token %q", required)
		}
	}
}

func TestChatOperationalListsConsumePaginatedServerBundles(t *testing.T) {
	t.Parallel()
	required := map[string][]string{
		"web/src/lib/chat_channel_options.ts": {
			"fetchChannelOptionsPage(", "page.html.bundle", "page.next_offset", "offset = page.next_offset",
		},
		"web/src/lib/chat_jobs_coordinator.ts":   {"fetchChatJobsPage(", "page.html.bundle", "next_offset"},
		"web/src/lib/chat_memory_coordinator.ts": {"fetchChatMemoryPage(", "page.html.bundle", "button.dataset.nextOffset"},
		"web/src/controllers/chat_view_controller.ts": {
			"fetchChatTimelinePage(", "page.html.bundle", "next_offset",
		},
	}
	for path, tokens := range required {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range tokens {
			if !strings.Contains(string(raw), token) {
				t.Errorf("%s lacks server component pagination token %q", path, token)
			}
		}
	}
	options := readFrontendSource(t, "web/src/lib/chat_channel_options.ts")
	if strings.Contains(options, "button.dataset.nextOffset") {
		t.Error("channel pagination still depends on the removed manual next-offset control")
	}
}
