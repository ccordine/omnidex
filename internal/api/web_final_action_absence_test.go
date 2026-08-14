package api

import (
	"os"
	"strings"
	"testing"
)

func TestFinalWebActionBoundariesHaveNoLooseAuthorityFallback(t *testing.T) {
	for path, forbidden := range map[string][]string{
		"scrum_flow_metrics_service.go": {
			"resolveProjectID(r)",
			"exactChannelQueryInteger",
		},
		"scrum_tags.go": {
			`settings["tags"]`,
			`settings["tags"].([]any)`,
		},
		"scrum_job_reference.go": {
			"source = strings.TrimSpace(source)",
			"value = strings.TrimSpace(value)",
		},
		"ui_project_tabs.go": {
			"uiMapStringList",
		},
		"web/src/controllers/shell_controller.ts": {
			"counts?.running || 0",
			"counts?.pending || 0",
			"counts?.waiting_input || 0",
		},
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, fragment := range forbidden {
			if strings.Contains(string(raw), fragment) {
				t.Errorf("%s retains loose authority fallback %q", path, fragment)
			}
		}
	}
	for _, path := range []string{
		"../projectgit/status.go",
		"../projectgit/command_exec.go",
		"../projectgit/status_marker.go",
		"../projectgit/status_parse.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(raw), "CombinedOutput") {
			t.Errorf("%s retains unbounded Git subprocess output collection", path)
		}
	}

	channelClient, err := os.ReadFile("web/src/lib/scrum_api.ts")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(channelClient), "readJSON<unknown>(response, SCRUM_CHANNEL_RESPONSE_MAX_BYTES)") != 2 {
		t.Fatal("Scrum channel GET and POST do not both enforce the shared encoded-response bound")
	}
	modalClient, err := os.ReadFile("web/src/lib/scrum_modal_api.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(modalClient), "readJSON<unknown>(response, SCRUM_CHANNEL_RESPONSE_MAX_BYTES)") {
		t.Fatal("Scrum modal channel window does not enforce the shared encoded-response bound")
	}
	for _, path := range []string{"web/src/lib/scrum_types.ts", "web/src/lib/scrum_board_tracker.ts"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(raw), "console_log") {
			t.Errorf("%s retains retired Scrum console-log compatibility state", path)
		}
	}

	developmentLoops, err := os.ReadFile("../../docs/DEVELOPMENT_LOOPS.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(developmentLoops), "recipe_required") {
		t.Fatal("development-loop documentation retains retired recipe authority")
	}
}
