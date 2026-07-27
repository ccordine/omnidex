package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIEntrypointOnlyDispatchesCommands(t *testing.T) {
	source := readCLISource(t, "main.go")
	if lines := strings.Count(source, "\n") + 1; lines > 140 {
		t.Fatalf("main.go has %d lines; the CLI entrypoint must stay at or below 140", lines)
	}
	for _, forbidden := range []string{
		"func runChat(",
		"func runEnqueue(",
		"func runWatch(",
		"func runMediaIndex(",
		"func printContextUpdates(",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("CLI entrypoint still owns command implementation %q", forbidden)
		}
	}
}

func TestCLIProductionFilesStayBelowGodFileThreshold(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read CLI package: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source := readCLISource(t, name)
		if lines := strings.Count(source, "\n") + 1; lines > 500 {
			t.Errorf("%s has %d lines; production CLI files must stay at or below 500", name, lines)
		}
	}
}

func TestSplitCLICommandFilesStayFocused(t *testing.T) {
	for _, path := range []string{
		"version_command.go",
		"enqueue_command.go",
		"chat_command.go",
		"chat_session.go",
		"chat_candidate.go",
		"chat_candidate_build.go",
		"chat_input.go",
		"chat_action_review.go",
		"chat_confirmation.go",
		"chat_capabilities.go",
		"chat_active_turn.go",
		"chat_repl.go",
		"job_query_commands.go",
		"memory_commands.go",
		"ingest_command.go",
		"feedback_command.go",
		"media_commands.go",
		"job_control_commands.go",
		"execution_profile.go",
		"cli_help.go",
		"watch_steps.go",
		"watch_context.go",
		"watch_events.go",
		"watch_llm_trace.go",
	} {
		source := readCLISource(t, path)
		if lines := strings.Count(source, "\n") + 1; lines > 350 {
			t.Errorf("%s has %d lines; split CLI command files must stay at or below 350", path, lines)
		}
	}
}

func readCLISource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
