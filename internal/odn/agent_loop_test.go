package odn

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAgentPlannerOutputCommands(t *testing.T) {
	commands, done, ask := parseAgentPlannerOutput("pwd\nls -la\n", defaultAgentCommandsPerStep)

	if done != "" {
		t.Fatalf("done = %q, want empty", done)
	}
	if ask != "" {
		t.Fatalf("ask = %q, want empty", ask)
	}
	if len(commands) != 2 {
		t.Fatalf("len(commands) = %d, want 2", len(commands))
	}
	if commands[0] != "pwd" || commands[1] != "ls -la" {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestParseAgentPlannerOutputDone(t *testing.T) {
	commands, done, ask := parseAgentPlannerOutput("DONE: objective complete", defaultAgentCommandsPerStep)

	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want empty", commands)
	}
	if ask != "" {
		t.Fatalf("ask = %q, want empty", ask)
	}
	if done != "objective complete" {
		t.Fatalf("done = %q, want objective complete", done)
	}
}

func TestParseAgentPlannerOutputAsk(t *testing.T) {
	commands, done, ask := parseAgentPlannerOutput("ASK: Which framework should I use?", defaultAgentCommandsPerStep)

	if len(commands) != 0 {
		t.Fatalf("commands = %#v, want empty", commands)
	}
	if done != "" {
		t.Fatalf("done = %q, want empty", done)
	}
	if ask != "Which framework should I use?" {
		t.Fatalf("ask = %q, want question", ask)
	}
}

func TestPolicyAllowsCurl(t *testing.T) {
	decision := EvaluateCommandPolicy("curl -s --max-time 20 https://example.com | grep -oP '(?<=<title>).*?(?=</title>)'", t.TempDir())

	if !decision.Allowed {
		t.Fatalf("curl blocked: %s %s", decision.ReasonCode, decision.Detail)
	}
}

func TestPolicyAllowsDate(t *testing.T) {
	decision := EvaluateCommandPolicy("date", t.TempDir())

	if !decision.Allowed {
		t.Fatalf("date blocked: %s %s", decision.ReasonCode, decision.Detail)
	}
}

func TestPolicyAllowsHomeProjectsPath(t *testing.T) {
	decision := EvaluateCommandPolicy("mkdir -p ~/Projects/odn-project", t.TempDir())

	if !decision.Allowed {
		t.Fatalf("home project mkdir blocked: %s %s", decision.ReasonCode, decision.Detail)
	}
}

func TestPolicyBlocksHomeOutsideProjects(t *testing.T) {
	decision := EvaluateCommandPolicy("mkdir ~/Desktop/odn-project", t.TempDir())

	if decision.Allowed {
		t.Fatal("home path outside ~/Projects should be blocked")
	}
	if decision.ReasonCode != "workspace_escape" {
		t.Fatalf("reason = %q, want workspace_escape", decision.ReasonCode)
	}
}

func TestPolicyBlocksCD(t *testing.T) {
	decision := EvaluateCommandPolicy("cd ~/Projects/odn-project", t.TempDir())

	if decision.Allowed {
		t.Fatal("cd should be blocked")
	}
	if decision.ReasonCode != "root_command_not_allowlisted" {
		t.Fatalf("reason = %q, want root_command_not_allowlisted", decision.ReasonCode)
	}
}

func TestPolicyDoesNotPhraseMatchPlaceholderAPIKey(t *testing.T) {
	decision := EvaluateCommandPolicy(`curl --max-time 20 "https://api.openweathermap.org/data/2.5/weather?q=Thailand&appid=YOUR_API_KEY"`, t.TempDir())

	if !decision.Allowed {
		t.Fatalf("placeholder-looking phrase should not be blocked by phrase matching: %s %s", decision.ReasonCode, decision.Detail)
	}
}

func TestAgentLoopCanUseSedToModifyExistingFileAndVerify(t *testing.T) {
	workspace := t.TempDir()
	settingsPath := filepath.Join(workspace, "settings.conf")
	if err := os.WriteFile(settingsPath, []byte("feature=disabled\nmode=dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	requests := []OllamaChatRequest{}
	client, closeServer := capturingOllamaClient(t, []string{
		"sed -n '1,120p' settings.conf",
		"sed -i 's/^feature=disabled$/feature=enabled/' settings.conf",
		"grep '^feature=enabled$' settings.conf",
		"DONE: settings.conf changed to feature=enabled and verified with grep",
	}, &requests)
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "sed-edit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "sed-edit-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"Read settings.conf, use sed to change feature=disabled to feature=enabled, then verify the file content.",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            5,
			MaxCommandsPerStep:  1,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	updated, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "feature=enabled") || strings.Contains(string(updated), "feature=disabled") {
		t.Fatalf("settings.conf not updated correctly:\n%s", string(updated))
	}
	if !transcriptCommandContains(result.Transcript, "sed -n '1,120p' settings.conf") {
		t.Fatalf("transcript missing file context read: %#v", result.Transcript)
	}
	if !transcriptCommandContains(result.Transcript, "sed -i 's/^feature=disabled$/feature=enabled/' settings.conf") {
		t.Fatalf("transcript missing sed edit: %#v", result.Transcript)
	}
	if !transcriptStdoutContains(result.Transcript, "feature=enabled") {
		t.Fatalf("transcript missing grep verification output: %#v", result.Transcript)
	}
	if len(requests) < 2 || !strings.Contains(joinOllamaMessageContent(requests[1].Messages), "feature=disabled") {
		t.Fatalf("second planner request should include observed file context: %#v", requests)
	}
}

func TestLatestObservationBlocksDone(t *testing.T) {
	if latestObservationBlocksDone([]CommandObservation{{Status: "success"}, {Status: "failed"}}) != true {
		t.Fatal("latest failed observation should block DONE")
	}
	if latestObservationBlocksDone([]CommandObservation{{Status: "failed"}, {Status: "success"}}) != false {
		t.Fatal("latest success observation should allow DONE")
	}
	if latestObservationBlocksDone([]CommandObservation{{Status: "blocked"}}) != true {
		t.Fatal("blocked observations should block DONE")
	}
	if latestObservationBlocksDone([]CommandObservation{{Status: "success"}, {Status: "blocked"}}) != true {
		t.Fatal("latest blocked observation should block DONE")
	}
}

func TestAgentPlannerPromptRejectsCD(t *testing.T) {
	prompt := buildAgentPlannerMessages("/tmp/work", "make a directory", nil, AgentCommandLoopConfig{MaxCommandsPerStep: 2})[0].Content

	if !strings.Contains(prompt, "No cd") {
		t.Fatalf("planner prompt missing cd rule:\n%s", prompt)
	}
}

func TestAgentPlannerPromptAllowsClarification(t *testing.T) {
	prompt := buildAgentPlannerMessages("/tmp/work", "make an app", nil, AgentCommandLoopConfig{MaxCommandsPerStep: 2})[0].Content

	if !strings.Contains(prompt, "ASK:") {
		t.Fatalf("planner prompt missing ASK rule:\n%s", prompt)
	}
}

func TestAgentLoopAsksClarificationAndInjectsAnswer(t *testing.T) {
	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		"ASK: Which project name should I use?",
		"mkdir chosen-name",
		"DONE: created chosen-name",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "clarification-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "clarification-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	out := &bytes.Buffer{}
	beforePrompt := 0
	afterPrompt := 0
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"create a project, name up for debate",
		PermissionFull,
		strings.NewReader("chosen-name\n"),
		out,
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            4,
			MaxCommandsPerStep:  2,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
			AllowClarification:  true,
			BeforeUserPrompt: func() {
				beforePrompt++
			},
			AfterUserPrompt: func() {
				afterPrompt++
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if !strings.Contains(out.String(), "Which project name should I use?") {
		t.Fatalf("clarification prompt missing question:\n%s", out.String())
	}
	if !transcriptCommandContains(result.Transcript, "ASK: Which project name should I use?") {
		t.Fatalf("transcript missing ASK observation: %#v", result.Transcript)
	}
	if !transcriptStdoutContains(result.Transcript, "chosen-name") {
		t.Fatalf("transcript missing injected answer: %#v", result.Transcript)
	}
	if !transcriptCommandContains(result.Transcript, "mkdir chosen-name") {
		t.Fatalf("transcript missing command after answer: %#v", result.Transcript)
	}
	if countEventsOfType(result.Events, "clarification_requested") != 1 {
		t.Fatalf("clarification_requested count mismatch: %#v", result.Events)
	}
	if countEventsOfType(result.Events, "clarification_answered") != 1 {
		t.Fatalf("clarification_answered count mismatch: %#v", result.Events)
	}
	if beforePrompt != 1 || afterPrompt != 1 {
		t.Fatalf("prompt hooks before=%d after=%d, want 1/1", beforePrompt, afterPrompt)
	}
}

func TestAgentLoopRejectsClarificationForExactCommand(t *testing.T) {
	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		"ASK: Confirm objective?",
		"pwd",
		"DONE: observed workspace",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "exact-command-ask-reject-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "exact-command-ask-reject-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"Run this exact command: pwd. Then finish from observed stdout.",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            4,
			MaxCommandsPerStep:  2,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
			AllowClarification:  false,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if countEventsOfType(result.Events, "clarification_rejected") != 1 {
		t.Fatalf("clarification_rejected count mismatch: %#v", result.Events)
	}
	if countEventsOfType(result.Events, "clarification_requested") != 0 {
		t.Fatalf("exact command should not prompt user: %#v", result.Events)
	}
	if !transcriptCommandContains(result.Transcript, "pwd") {
		t.Fatalf("pwd missing from transcript: %#v", result.Transcript)
	}
}

func TestAgentLoopRejectsClarificationForAnswerableFact(t *testing.T) {
	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		"ASK: What specific timezone does Virginia use?",
		"env TZ=America/New_York date '+%Y-%m-%d %H:%M:%S %Z %z'",
		"DONE: observed Virginia time",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "answerable-fact-ask-reject-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "answerable-fact-ask-reject-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	out := &bytes.Buffer{}
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"what time is it right now in Virginia?",
		PermissionFull,
		strings.NewReader(""),
		out,
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            4,
			MaxCommandsPerStep:  2,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
			AllowClarification:  false,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if strings.Contains(out.String(), "clarify>") {
		t.Fatalf("answerable fact should not prompt user:\n%s", out.String())
	}
	if countEventsOfType(result.Events, "clarification_rejected") != 1 {
		t.Fatalf("clarification_rejected count mismatch: %#v", result.Events)
	}
	if !transcriptCommandContains(result.Transcript, "env TZ=America/New_York date") {
		t.Fatalf("timezone command missing: %#v", result.Transcript)
	}
	if !hasObservedOutput(result.Transcript) {
		t.Fatalf("timezone command produced no output: %#v", result.Transcript)
	}
}

func TestAgentLoopRejectsDoneForFactQuestionWithoutOutput(t *testing.T) {
	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		"touch empty-result",
		"DONE: 2023-04-15T12:34:56",
		"printf 'verified-output\\n'",
		"DONE: verified-output",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "empty-output-done-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "empty-output-done-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"what time is it right now?",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            5,
			MaxCommandsPerStep:  2,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
			RequireEvidence:     true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if countEventsOfType(result.Events, "planner_done_rejected") == 0 {
		t.Fatalf("expected DONE rejection for empty evidence: %#v", result.Events)
	}
	if !transcriptStdoutContains(result.Transcript, "verified-output") {
		t.Fatalf("expected recovery command output: %#v", result.Transcript)
	}
}

func TestAgentLoopRejectsNullOutputForWeatherQuestion(t *testing.T) {
	original := evidenceCommandsForObjective
	evidenceCommandsForObjective = func(string) []string { return nil }
	defer func() {
		evidenceCommandsForObjective = original
	}()

	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		"printf 'null\\n'",
		"DONE: clear sky 29C",
		"printf 'temp_C=30 weather=Partly Cloudy humidity=66\\n'",
		"DONE: temp_C=30 weather=Partly Cloudy humidity=66",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "weather-null-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "weather-null-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"what is the weather right now in Thailand?",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            5,
			MaxCommandsPerStep:  2,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
			RequireEvidence:     true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if result.FailedCount == 0 {
		t.Fatalf("null output should count as failed evidence: %#v", result)
	}
	if !transcriptStdoutContains(result.Transcript, "temp_C=30") {
		t.Fatalf("expected recovery weather evidence: %#v", result.Transcript)
	}
}

func TestCommandBatchStopsAfterBlockedCommand(t *testing.T) {
	workspace := t.TempDir()
	client, closeServer := fakeOllamaClient(t, []string{
		"mkdir subdir\ncd subdir\ntouch README.md\ngit init",
		"touch subdir/README.md\ngit -C subdir init",
		"DONE: created subdir project",
	})
	defer closeServer()

	runLogger, err := NewRunLogger(t.TempDir(), "batch-stop-test")
	if err != nil {
		t.Fatal(err)
	}
	defer runLogger.Close()

	session := &Session{
		WorkspacePath: workspace,
		WorkspaceHash: "batch-stop-test",
		Permission:    PermissionFull,
	}
	nextID := 0
	result, err := ExecuteAgentCommandLoopWithConfig(
		context.Background(),
		session,
		"create a project in subdir",
		PermissionFull,
		strings.NewReader(""),
		&bytes.Buffer{},
		client,
		func() string {
			nextID++
			return fmt.Sprintf("evt_%03d", nextID)
		},
		runLogger,
		AgentCommandLoopConfig{
			MaxSteps:            4,
			MaxCommandsPerStep:  4,
			MaxObservationChars: 1000,
			PlannerTimeout:      5 * time.Second,
			CommandTimeout:      5 * time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done {
		t.Fatalf("done = false; result=%#v", result)
	}
	if !transcriptCommandContains(result.Transcript, "cd subdir") {
		t.Fatalf("blocked cd missing from transcript: %#v", result.Transcript)
	}
	if transcriptHasExactCommand(result.Transcript, "touch README.md") {
		t.Fatalf("batch continued after blocked cd; transcript=%#v", result.Transcript)
	}
	if transcriptHasExactCommand(result.Transcript, "git init") {
		t.Fatalf("batch continued to workspace git init after blocked cd; transcript=%#v", result.Transcript)
	}
	if !transcriptCommandContains(result.Transcript, "touch subdir/README.md") {
		t.Fatalf("recovery command missing: %#v", result.Transcript)
	}
}

func transcriptHasExactCommand(transcript []CommandObservation, command string) bool {
	for _, obs := range transcript {
		if obs.Command == command {
			return true
		}
	}
	return false
}

func countEventsOfType(events []Event, eventType string) int {
	count := 0
	for _, evt := range events {
		if evt.Type == eventType {
			count++
		}
	}
	return count
}
