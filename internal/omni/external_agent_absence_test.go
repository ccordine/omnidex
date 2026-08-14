package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalWholeWorkspaceAgentSubsystemIsAbsent(t *testing.T) {
	for _, path := range []string{
		"codex_sdk_agent.go", "cursor_sdk_agent.go", "external_agent_command_session.go",
		"external_agent_hostbridge.go", "external_agent_session.go", "external_agent_stream.go",
		"external_coding_prompt.go", "external_sdk_enable.go",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired whole-workspace external agent source remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	for _, pattern := range []string{"../codexrunner/*.go", "../cursorrunner/*.go"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Errorf("retired whole-workspace external agent source remains: %v", matches)
		}
	}
	raw, err := os.ReadFile("value_helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "externalAgentTimeout") {
		t.Fatal("retired external-agent timeout configuration remains")
	}
	for _, path := range []string{"../../AGENTS.md", "../../CHANGELOG.md"} {
		document, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"external-agent configuration", "runtime-backed model, external-agent"} {
			if strings.Contains(string(document), forbidden) {
				t.Errorf("%s advertises retired execution authority %q", path, forbidden)
			}
		}
	}
}
