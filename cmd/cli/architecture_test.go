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
		"chat_input.go",
		"chat_core_turn.go",
		"chat_active_turn.go",
		"chat_repl.go",
		"job_query_commands.go",
		"memory_commands.go",
		"ingest_command.go",
		"feedback_command.go",
		"media_commands.go",
		"job_control_commands.go",
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

func TestInteractiveChatHasNoPhraseBasedLocalAutomationRouter(t *testing.T) {
	for _, path := range []string{
		"action_router.go",
		"chat_candidate.go",
		"chat_candidate_build.go",
		"chat_action_review.go",
		"chat_confirmation.go",
		"chat_capabilities.go",
		"audio_notes_chat_local.go",
		"media_local.go",
		"media_playback.go",
		"shell_intent.go",
		"shell_file_intent.go",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("phrase-based interactive router must be absent: %s", path)
		}
	}

	source := readCLISource(t, "chat_command.go")
	for _, forbidden := range []string{
		"buildChatActionCandidate",
		"executeConfirmedChatAction",
		"interpretConfirmationReply",
		"--local-media",
		"--local-browser",
		"--local-screen",
		"--local-shell",
		"--local-audio",
		"--confirm-actions",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("interactive chat still contains forbidden local routing token %q", forbidden)
		}
	}
}

func TestCLIHasNoHeuristicResearchSidecar(t *testing.T) {
	for _, path := range []string{
		"research_local.go",
		"research_storage.go",
		"research_documents.go",
		"research_dossier.go",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("heuristic research sidecar must be absent: %s", path)
		}
	}
	if source := readCLISource(t, "main.go"); strings.Contains(source, `case "research":`) {
		t.Error("CLI entrypoint still exposes the removed research sidecar")
	}
	if source := readCLISource(t, "cli_help.go"); strings.Contains(source, "  research [") {
		t.Error("CLI help still advertises the removed research sidecar")
	}
}

func TestCLIHasNoWriteOnlyAgentControls(t *testing.T) {
	if _, err := os.Stat("execution_profile.go"); !os.IsNotExist(err) {
		t.Fatal("execution_profile.go must be absent; it only configured metadata the runtime never consumed")
	}

	paths := []string{
		"chat_command.go",
		"enqueue_command.go",
		"chat_repl.go",
		"chat_runtime_settings.go",
		"chat_ui.go",
		"cli_help.go",
	}
	forbidden := []string{
		"persistent_execution",
		"planning_passes",
		"review_always",
		"allow_missing_tools",
		"reasoning_level",
		"autonomy_mode",
		"approval_mode",
		"verification_mode",
		"verification_iterations",
		"architect_mode",
		`fs.String("profile"`,
		`fs.String("web"`,
		`fs.String("workspace"`,
		`fs.Bool("allow-missing-tools"`,
		`fs.String("reasoning"`,
		`fs.String("autonomy"`,
		`fs.String("approval"`,
		`fs.String("verify"`,
		`fs.Int("verify-iterations"`,
		"--profile default|architect",
		"--web auto|on|off",
		"--workspace auto|on|off",
		"--allow-missing-tools",
		"--reasoning auto|fast|deep",
		"--autonomy auto|on|off",
		"--approval auto|on|off",
		"--verify auto|on|off",
		"--verify-iterations 1-4",
	}
	for _, path := range paths {
		source := readCLISource(t, path)
		for _, token := range forbidden {
			if strings.Contains(source, token) {
				t.Errorf("%s still exposes write-only agent control %q", path, token)
			}
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
