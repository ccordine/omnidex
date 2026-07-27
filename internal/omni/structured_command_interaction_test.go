package omni

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredCommandResponseStreamsUseCurrentCommandObservationOnFailure(t *testing.T) {
	result := CommandDecisionResult{
		Command:  "find . -maxdepth 1 -type f",
		ExitCode: 0,
		Observations: []StructuredCommandObservation{
			{CommandID: "cmd_install", Command: "npm install", ExitCode: 0, Stdout: "added 15 packages\n"},
			{CommandID: "cmd_find", Command: "find . -maxdepth 1 -type f", ExitCode: 0, Stdout: "./package.json\n"},
		},
	}
	stdout, stderr := structuredCommandResponseStreams(result, "added 15 packages\n./package.json\n", "vite: command not found\n", context.Canceled)
	if strings.Contains(stdout, "added 15 packages") || strings.Contains(stderr, "vite: command not found") {
		t.Fatalf("response streams used aggregate output: stdout=%q stderr=%q", stdout, stderr)
	}
	if stdout != "./package.json\n" {
		t.Fatalf("stdout = %q, want current find output", stdout)
	}
}

func TestValidateStructuredCommandRejectsSemicolonMutationChains(t *testing.T) {
	workspace := t.TempDir()
	for _, command := range []string{
		"mv src/App.js src/App.jsx; mv src/main.js src/main.jsx",
		"npm install vite; npm run build",
		"cat > src/App.jsx; npm run build",
		"node -e \"require('fs').writeFileSync('src/App.jsx', 'x')\"; npm run build",
		"python -c \"from pathlib import Path; Path('src/App.jsx').write_text('x')\"; npm run build",
		"cd app; npm run build",
	} {
		if err := validateStructuredCommandForRun(command, nil, workspace, nil); err == nil {
			t.Fatalf("command %q should be rejected", command)
		}
	}
}

func TestValidateStructuredCommandRejectsWildcardMutationLoops(t *testing.T) {
	workspace := t.TempDir()
	command := "for f in src/*.js; do mv \"$f\" \"${f%.js}.jsx\"; done"
	if err := validateStructuredCommandForRun(command, nil, workspace, nil); err == nil {
		t.Fatalf("wildcard mutation loop should be rejected")
	}
}

func TestStructuredCommandClassifiesFirstMoveFailureWhenSecondMutationSucceeds(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := "sh -lc 'mv src/Missing.js src/Missing.jsx; printf ok > src/App.jsx; true'"
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 || len(result.Observations) == 0 || result.Observations[0].ExitCode == 0 {
		t.Fatalf("partial failure was not converted to failed evidence: %#v", result)
	}
	if !fileExists(filepath.Join(workspace, "src", "App.jsx")) {
		t.Fatalf("second mutation did not run; test setup invalid")
	}
	if !strings.Contains(result.Observations[0].Stderr, "partial_failure") {
		t.Fatalf("stderr missing partial failure marker: %#v", result.Observations[0])
	}
	if !structuredEventsContain(events, "structured_command_partial_failure_classified") {
		t.Fatalf("missing partial failure event: %#v", events)
	}
}

func TestCannotStatPreventsObjectiveSatisfaction(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.jsx"), []byte("export default function App(){ return <main /> }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := []StructuredObjective{{
		ID:               "rename_app_js_to_jsx",
		Description:      "Rename App.js to App.jsx",
		Status:           "pending",
		RequiredEvidence: []string{"file_exists:src/App.jsx", "file_absent:src/App.js"},
	}}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "mv src/App.js src/App.jsx",
		ExitCode: 1,
		Stderr:   "mv: cannot stat 'src/App.js': No such file or directory",
		CWD:      workspace,
	}, nil)
	if structuredObjectiveSatisfied(updated[0]) {
		t.Fatalf("cannot stat failure should not satisfy objective: %#v", updated)
	}
}

func TestStructuredCommandClassifiesPartialFailureEvenWithZeroExit(t *testing.T) {
	workspace := t.TempDir()
	command := "sh -lc 'echo \"mv: cannot stat src/App.js: No such file or directory\" >&2; true'"
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 || len(result.Observations) == 0 || result.Observations[0].ExitCode == 0 {
		t.Fatalf("partial failure was not converted to failed evidence: %#v", result)
	}
	if !strings.Contains(result.Observations[0].Stderr, "partial_failure") {
		t.Fatalf("stderr missing partial failure marker: %#v", result.Observations[0])
	}
	if !structuredEventsContain(events, "structured_command_partial_failure_classified") {
		t.Fatalf("missing partial failure event: %#v", events)
	}
}

func TestValidateStructuredCommandRejectsBareViteWhenNpmScriptShouldBeUsed(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"scripts":{"build":"vite build"},"devDependencies":{"vite":"latest"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateStructuredCommandForRunWithArchitect("vite build", "build the Vite app", "", "", nil, workspace, nil, WorksiteSurvey{PackageManager: packageManagerNPM})
	if err == nil || !strings.Contains(err.Error(), "prefer npm scripts") {
		t.Fatalf("err = %v, want bare vite rejection", err)
	}
	if err := validateStructuredCommandForRunWithArchitect("npm run build", "build the Vite app", "", "", nil, workspace, nil, WorksiteSurvey{PackageManager: packageManagerNPM}); err != nil {
		t.Fatalf("npm run build should be allowed: %v", err)
	}
}

func TestStructuredCommandDecisionIncludesRecentConversationForFollowups(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'Pattaya rain chance from history\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Using prior location Pattaya, Thailand."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	history := []Message{
		{Role: "user", Content: "what is the weather in Pattaya Thailand today?"},
		{Role: "assistant", Content: "The weather in Pattaya, Thailand today is Partly Cloudy with temperatures ranging from +31°C to +36°C."},
	}

	result, err := RunStructuredCommandDecisionWithHistoryEventsAndAsk(
		context.Background(),
		"Will it rain there today?",
		history,
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			t.Fatalf("should use recent conversation instead of asking: %q", question)
			return "", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests[0].Messages) != 1 {
		t.Fatalf("messages = %#v, want active task only without interpreter-approved history", client.requests[0].Messages)
	}
	if strings.Contains(client.requests[0].Messages[0].Content, "Pattaya") {
		t.Fatalf("active task should not contain copied reference location without interpreter approval: %s", client.requests[0].Messages[0].Content)
	}
	if !strings.Contains(stdout.String(), "Pattaya rain chance") {
		t.Fatalf("history-resolved command did not run from fake planner response: stdout=%q", stdout.String())
	}
	if !strings.Contains(result.Answer, "Pattaya") {
		t.Fatalf("answer should preserve resolved location from observed evidence: %q", result.Answer)
	}
}

func TestStructuredCommandDecisionIncludesInterpreterApprovedRecentConversationForFollowups(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'Pattaya rain chance from history\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Using prior location Pattaya, Thailand."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	history := []Message{
		{Role: "user", Content: "what is the weather in Pattaya Thailand today?"},
		{Role: "assistant", Content: "The weather in Pattaya, Thailand today is Partly Cloudy with temperatures ranging from +31°C to +36°C."},
	}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RequiresReferenceHistory: true,
		ObjectiveLedger: []StructuredObjective{requiredCommandObjectiveForTest(
			"report_recent_conversation_weather",
			"Report weather evidence using approved recent conversation",
			"printf",
		)},
	}}}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"Will it rain there today?",
		history,
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			t.Fatalf("should use interpreter-approved recent conversation instead of asking: %q", question)
			return "", nil
		},
		structuredCommandDecisionRunConfig{PromptInterpreter: interpreter},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests[0].Messages) != 3 {
		t.Fatalf("messages = %#v, want reference history plus active task", client.requests[0].Messages)
	}
	if !strings.Contains(client.requests[0].Messages[0].Content, "reference_history") || !strings.Contains(client.requests[0].Messages[0].Content, "Pattaya") {
		t.Fatalf("structured request missing conversation history: %#v", client.requests[0].Messages)
	}
	if strings.Contains(client.requests[0].Messages[2].Content, "Pattaya") {
		t.Fatalf("active task should not contain copied reference location: %s", client.requests[0].Messages[2].Content)
	}
	if !strings.Contains(stdout.String(), "Pattaya rain chance") {
		t.Fatalf("history-resolved command did not run: stdout=%q", stdout.String())
	}
	if !strings.Contains(result.Answer, "Pattaya") {
		t.Fatalf("answer should preserve resolved location: %q", result.Answer)
	}
}

func TestStructuredCommandDecisionRejectsPlaceholderAngleBracketCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"curl -s wttr.in/<location> | grep Sunny","done":false,"answer":""}`,
		`{"command":"printf 'used concrete location\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"used concrete location"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "The weather where will be sunny?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want placeholder rejection and retry", client.calls)
	}
	if len(result.Observations) < 2 || !strings.Contains(result.Observations[0].Stderr, "placeholder angle-bracket") {
		t.Fatalf("first observation should reject placeholder command: %#v", result.Observations)
	}
	if strings.Contains(stderr.String(), "syntax error") {
		t.Fatalf("placeholder command reached bash: stderr=%q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "used concrete location") {
		t.Fatalf("retry command did not run: stdout=%q", stdout.String())
	}
}

func TestStructuredCommandRejectsLiteralPlaceholderProjectPath(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"cd /path/to/project && printf 'bad\n' > src/App.js","done":false,"answer":""}`,
		`{"command":"printf 'used real workspace\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"used real workspace"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEvents(
		context.Background(),
		"Update the app source file",
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want placeholder rejection and retry", client.calls)
	}
	if len(result.Observations) < 2 || !strings.Contains(result.Observations[0].Stderr, "placeholder project path") {
		t.Fatalf("first observation should reject literal placeholder path: %#v", result.Observations)
	}
	if !structuredEventsContain(events, "structured_command_rejected_placeholder_path") {
		t.Fatalf("missing placeholder path rejection event: %#v", events)
	}
	if strings.Contains(stderr.String(), "/path/to/project") {
		t.Fatalf("placeholder command reached shell: stderr=%q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "used real workspace") {
		t.Fatalf("retry command did not run: stdout=%q", stdout.String())
	}
}

func TestStructuredCommandDecisionRejectsAskBeforeCommandObservation(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","ask":true,"question":"Should I inspect this system?"}`,
		`{"command":"printf 'inspected\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"inspected"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	asked := false

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Inspect this system.",
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			asked = true
			return "yes", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if asked {
		t.Fatal("ask callback should not be called before command evidence")
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejected ask + command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "ask rejected") {
		t.Fatalf("first observation should reject premature ask: %#v", result.Observations[0])
	}
	if result.Answer != "inspected" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionRejectsAskAfterSuccessfulCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":false,"answer":"","ask":true,"question":"Should I continue?"}`,
		`{"command":"","done":true,"answer":"evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	asked := false

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Use evidence.",
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			asked = true
			return "yes", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if asked {
		t.Fatal("ask callback should not be called after successful command")
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want command + rejected ask", result.Observations)
	}
	if result.Observations[1].Command != "" || !strings.Contains(result.Observations[1].Stderr, "latest real command succeeded") {
		t.Fatalf("second observation should reject ask after success: %#v", result.Observations[1])
	}
	if result.Answer != "evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionAskWithoutHandlerRequiresUserInput(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-needs-sudo","done":false,"answer":""}`,
		`{"command":"","done":false,"answer":"","ask":true,"question":"Need sudo approval."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := RunStructuredCommandDecision(context.Background(), "Install a protected tool.", client, stdout, stderr)
	if err == nil {
		t.Fatal("expected user input required error")
	}
	var inputErr UserInputRequiredError
	if !errors.As(err, &inputErr) {
		t.Fatalf("err = %T %v, want UserInputRequiredError", err, err)
	}
	if inputErr.Question != "Need sudo approval." {
		t.Fatalf("question = %q", inputErr.Question)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "omni-nxt-needs-sudo") {
		t.Fatalf("unexpected command output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionEmitsRealtimeEvents(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'event evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"event evidence"}`,
	}}
	events := []StructuredCommandEvent{}

	_, err := RunStructuredCommandDecisionWithEvents(
		context.Background(),
		"produce event evidence",
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{
		"worksite_survey_completed",
		"structured_llm_request_started",
		"structured_llm_payload_received",
		"structured_command_started",
		"structured_command_finished",
		"structured_llm_request_started",
		"structured_llm_payload_received",
		"completion_check_accepted_from_done_request",
	}
	if len(events) != len(wantOrder) {
		t.Fatalf("events=%#v want %d", events, len(wantOrder))
	}
	for i, want := range wantOrder {
		if events[i].Type != want {
			t.Fatalf("event %d = %s, want %s; events=%#v", i, events[i].Type, want, events)
		}
	}
}

func TestStructuredCommandDecisionRejectsDoneBeforeCommandObservation(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":true,"answer":"/home/user"}`,
		`{"command":"printf '/real/workdir\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"/real/workdir"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "Where am I in the filesystem?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want 3", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejection + command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "done rejected") {
		t.Fatalf("first observation should reject premature done: %#v", result.Observations[0])
	}
	if result.Command != "printf '/real/workdir\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "/real/workdir" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if stdout.String() != "/real/workdir\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStructuredCommandDecisionRejectsRepeatedDoneWithoutRealCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":true,"answer":"use a weather website"}`,
		`{"command":"","done":true,"answer":"still no command"}`,
		`{"command":"printf 'public weather evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"public weather evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "what is the weather in Thailand right now?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 4 {
		t.Fatalf("llm calls = %d, want 4", client.calls)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want two rejections + command", result.Observations)
	}
	if result.Observations[0].Command != "" || result.Observations[1].Command != "" {
		t.Fatalf("first two observations should be done rejections: %#v", result.Observations)
	}
	if result.Command != "printf 'public weather evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "public weather evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionRejectsPureEchoAnswerAsEvidence(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"echo 'I do not have access to real-time weather. Check a weather website.'","done":false,"answer":""}`,
		`{"command":"printf 'Virginia weather evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Virginia weather evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What is the weather in Virginia right now?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want rejected echo then command then done", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejected echo + real command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "pure echo command is not command evidence") {
		t.Fatalf("first observation should reject pure echo answer: %#v", result.Observations[0])
	}
	if strings.Contains(stdout.String(), "I do not have access") {
		t.Fatalf("fake answer command should not execute: stdout=%q", stdout.String())
	}
	if result.Command != "printf 'Virginia weather evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "Virginia weather evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionRejectsLeadingRedirectArtifact(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":">> echo 'I do not have access to real-time information.'","done":false,"answer":""}`,
		`{"command":"printf 'Pattaya time evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Pattaya time evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What time is it in Pattaya right now?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want rejected redirect then command then done", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejected redirect + real command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "command starts with shell redirection token") {
		t.Fatalf("first observation should reject leading redirect artifact: %#v", result.Observations[0])
	}
	if stdout.String() != "Pattaya time evidence\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if result.Answer != "Pattaya time evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionValidatesNonEmptyDoneCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"echo 'I do not have access to real-time information. Check the current time with a time zone app.'","done":true,"answer":"I cannot check."}`,
		`{"command":"printf 'Pattaya time evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Pattaya time evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What time is it in Pattaya right now?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want invalid done command then command then done", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejected echo + real command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "pure echo command is not command evidence") {
		t.Fatalf("first observation should reject non-empty done echo command: %#v", result.Observations[0])
	}
	if strings.Contains(stdout.String(), "I do not have access") {
		t.Fatalf("fake done command should not execute: stdout=%q", stdout.String())
	}
	if result.Answer != "Pattaya time evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionAcceptsDoneWithRepeatedSuccessfulCommand(t *testing.T) {
	command := "printf 'Pattaya time evidence\n'"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForTest(command) + `,"done":false,"answer":""}`,
		`{"command":` + quoteJSONForTest(command) + `,"done":true,"answer":""}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEvents(
		context.Background(),
		"What time is it in Pattaya right now?",
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatalf("llm calls = %d, want command then done", client.calls)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %#v, want only one command execution", result.Observations)
	}
	if stdout.String() != "Pattaya time evidence\n" {
		t.Fatalf("stdout = %q, want one command output", stdout.String())
	}
	if result.Answer != "Pattaya time evidence" {
		t.Fatalf("answer = %q, want synthesized stdout evidence", result.Answer)
	}
	if !structuredEventsContain(events, "completion_check_accepted_from_done_request") {
		t.Fatalf("missing validator completion accepted event: %#v", events)
	}
}

func TestStructuredCommandDecisionRejectsFalseCapabilityFinalAnswer(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'Saipan news evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"I'm sorry, but I can't provide real-time news updates."}`,
		`{"command":"","done":true,"answer":"Saipan news evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What are the current events in Saipan?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want command, rejected done, accepted done", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want command + rejected done", result.Observations)
	}
	if !strings.Contains(result.Observations[1].Stderr, "final answer claims inability") {
		t.Fatalf("second observation should reject false limitation: %#v", result.Observations[1])
	}
	if result.Answer != "Saipan news evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionRejectsDeferredEvidenceFinalAnswer(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"cat /etc/os-release","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"The architecture can be determined by running uname -m."}`,
		`{"command":"uname -m","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Architecture evidence gathered."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "Identify this machine's operating system and architecture.", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 4 {
		t.Fatalf("llm calls = %d, want command, rejected deferred done, command, done", client.calls)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want two commands + rejected done", result.Observations)
	}
	if !strings.Contains(result.Observations[1].Stderr, "final answer describes commands that should be run") {
		t.Fatalf("second observation should reject deferred command answer: %#v", result.Observations[1])
	}
	if !strings.Contains(stdout.String(), "\n") {
		t.Fatalf("stdout should include command output: %q", stdout.String())
	}
}
