package omni

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStructuredDependencyScopeAllowsDetectedProjectPackages(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"tailwindcss":"latest"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateStructuredCommandForRun("npm install tailwindcss", nil, workspace, nil); err != nil {
		t.Fatalf("detected existing dependency should be allowed: %v", err)
	}
}

func TestMemorySuggestedObjectiveDoesNotBlockCompletion(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "react_project", Description: "Create React project", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "tailwind_preference", Description: "User likes Tailwind", Status: "pending", Source: structuredObjectiveSourceMemorySuggested, Required: false, Packages: []string{"tailwindcss"}},
	}
	if pending := pendingStructuredObjectives(ledger); len(pending) != 0 {
		t.Fatalf("memory-suggested optional objective should not block completion: %#v", pending)
	}
}

func TestStructuredCommandDecisionAllowsReferenceHistoryForInterpreterMarkedFollowup(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'Pattaya rain evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Pattaya rain evidence"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RequiresReferenceHistory: true,
		ObjectiveLedger: []StructuredObjective{requiredCommandObjectiveForTest(
			"report_followup_weather",
			"Report weather evidence using approved reference history",
			"printf",
		)},
	}}}
	history := []Message{
		{Role: "user", Content: "What is the weather in Pattaya today?"},
		{Role: "assistant", Content: "Pattaya weather evidence."},
	}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"Will it rain there?",
		history,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		nil,
		structuredCommandDecisionRunConfig{PromptInterpreter: interpreter},
	)
	if err != nil {
		t.Fatalf("%v; result=%#v", err, result)
	}
	firstRequest := joinOllamaMessageContent(client.requests[0].Messages)
	if !strings.Contains(firstRequest, "reference_history") || !strings.Contains(firstRequest, "Pattaya") {
		t.Fatalf("follow-up did not include interpreter-approved reference history: %s", firstRequest)
	}
}

func TestObjectiveLedgerNeedsSubstantiveAppFilesDoesNotMatchApprovedSubstring(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:          "report_followup_weather",
		Description: "Report weather evidence using approved reference history",
		Status:      "pending",
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
	}}
	if objectiveLedgerNeedsSubstantiveAppFiles(ledger) {
		t.Fatalf("approved reference history was misclassified as an app objective: %#v", ledger)
	}
}

func TestStructuredReferenceHistoryOmitsPriorOperationalLoopState(t *testing.T) {
	history := []Message{
		{Role: "assistant", Content: strings.Join([]string{
			"Result",
			"Command: npm install react react-dom tailwindcss",
			"Last command exit code: 1",
			"Stdout: old install output",
			"Stderr: old install failure",
			"Status:",
			"  Pending objectives: setup_tailwind_css",
			"  Loop blocker: anti_loop: command rejected again",
			"  Forbidden command(s): npm install react react-dom tailwindcss",
			"  progression_gate_failed repeated command exhausted",
			"Useful summary: prior run stopped after inspecting package.json.",
		}, "\n")},
		{Role: "user", Content: "Build a React clock app here."},
	}

	message := buildStructuredCommandHistoryMessage(history)
	for _, leaked := range []string{
		"npm install react react-dom tailwindcss",
		"Loop blocker",
		"Forbidden command",
		"anti_loop",
		"progression_gate",
		"Pending objectives",
		"Last command exit code",
		"old install output",
		"old install failure",
	} {
		if strings.Contains(message, leaked) {
			t.Fatalf("reference history leaked prior operational state %q: %s", leaked, message)
		}
	}
	if !strings.Contains(message, "Useful summary: prior run stopped after inspecting package.json.") {
		t.Fatalf("reference history removed non-operational summary: %s", message)
	}
	if !strings.Contains(message, "Build a React clock app here.") {
		t.Fatalf("reference history removed user context: %s", message)
	}
}

func TestStructuredRuntimeRepeatStateIsDiagnosticAndCurrentRunScoped(t *testing.T) {
	command := "npm install react react-dom tailwindcss"
	currentRunObservations := []StructuredCommandObservation{
		{Step: 1, Command: command, ExitCode: 1, Stderr: "npm failed"},
	}
	if repeatedFailedStructuredCommand(command, currentRunObservations) {
		t.Fatal("failed commands are evidence for correction, not deterministic repeat bans")
	}
	if err := validateStructuredCommandForObservations(command, currentRunObservations); err != nil {
		t.Fatalf("same command in current run should remain executable while repeat state is diagnostic, err=%v", err)
	}
	if err := validateStructuredCommandForObservations(command, nil); err != nil {
		t.Fatalf("same command in a new run should not inherit blockers, err=%v", err)
	}

	message := buildStructuredCommandUserMessage("Build a React clock app.", nil, t.TempDir())
	for _, want := range []string{
		`"runtime_state_lifetime"`,
		`"completed_actions":"current_structured_run_only"`,
		`"forbidden_commands":"empty_by_default_not_derived_from_observations"`,
		`"command_cache":"persistent_advisory_evidence_not_policy"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing runtime lifetime marker %q: %s", want, message)
		}
	}
}

func TestStructuredCommandDecisionFailsWithoutLLM(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := RunStructuredCommandDecision(context.Background(), "Where am I in the filesystem?", nil, stdout, stderr)
	if err == nil {
		t.Fatal("expected missing LLM client to fail")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("command executed without llm: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionFailsBeforeExecutionWhenLLMResponseInvalid(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{`{"not_command":"pwd"}`}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := RunStructuredCommandDecision(context.Background(), "Where am I in the filesystem?", client, stdout, stderr)
	if err == nil {
		t.Fatal("expected invalid structured payload to fail")
	}
	if client.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", client.calls)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("command executed from invalid llm response: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionRetriesTransientOllamaRunnerStop(t *testing.T) {
	client := &fakeCommandDecisionClient{
		errors: []error{
			fmt.Errorf(`ollama returned status 500: {"error":"model runner has unexpectedly stopped"}`),
			nil,
			nil,
		},
		responses: []string{
			`{"command":"printf 'recovered\n'","done":false,"answer":""}`,
			`{"command":"","done":true,"answer":"recovered"}`,
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEvents(context.Background(), "Recover from transient model failure.", client, stdout, stderr, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want retry then command/done", client.calls)
	}
	if !strings.Contains(stdout.String(), "recovered") || result.Answer != "recovered" {
		t.Fatalf("unexpected result stdout=%q answer=%q", stdout.String(), result.Answer)
	}
	if !structuredEventsContain(events, "structured_llm_request_failed") || !structuredEventsContain(events, "structured_llm_request_recovered") {
		t.Fatalf("missing retry events: %#v", events)
	}
	if !structuredEventsContain(events, "structured_llm_backend_unstable") {
		t.Fatalf("missing backend instability event: %#v", events)
	}
	unstable := structuredEventOfTypeForTest(events, "structured_llm_backend_unstable")
	if unstable.Details["backoff"] == "" {
		t.Fatalf("backend instability event missing backoff: %#v", unstable)
	}
}

func TestStructuredLLMRetryBackoffIsExponentialAndBounded(t *testing.T) {
	if got := structuredLLMRetryBackoff(1); got != 2*time.Second {
		t.Fatalf("attempt 1 backoff = %s", got)
	}
	if got := structuredLLMRetryBackoff(2); got != 4*time.Second {
		t.Fatalf("attempt 2 backoff = %s", got)
	}
	if got := structuredLLMRetryBackoff(10); got != maxStructuredLLMBackoff {
		t.Fatalf("attempt 10 backoff = %s", got)
	}
}

func TestClassifyStructuredLLMFailureIdentifiesRunnerCrash(t *testing.T) {
	err := fmt.Errorf(`ollama returned status 500: {"error":"model runner has unexpectedly stopped"}`)
	if got := classifyStructuredLLMFailure(err); got != "ollama_model_runner_crash_or_restart" {
		t.Fatalf("diagnosis = %q", got)
	}
}

func TestStructuredCommandDefaultTimeoutAllowsLongRunningAgenticWork(t *testing.T) {
	if defaultOllamaRequestTimeout != 10*time.Minute {
		t.Fatalf("ollama request timeout = %s, want 10m", defaultOllamaRequestTimeout)
	}
	if defaultStructuredEvaluatorTimeout != defaultOllamaRequestTimeout {
		t.Fatalf("evaluator timeout = %s, want ollama request timeout %s", defaultStructuredEvaluatorTimeout, defaultOllamaRequestTimeout)
	}
	if defaultCommandDecisionTimeout != 6*time.Hour {
		t.Fatalf("command decision timeout = %s, want 6h long-running task budget", defaultCommandDecisionTimeout)
	}
	if defaultCommandDecisionMaxSteps < 40 {
		t.Fatalf("max structured steps = %d, want enough steps for multi-objective app builds", defaultCommandDecisionMaxSteps)
	}
}

func TestStructuredCommandDecisionLLMFailureBeforeCommandSetsExitCodeOne(t *testing.T) {
	client := &fakeCommandDecisionClient{
		errors: []error{
			fmt.Errorf(`ollama returned status 500: {"error":"model runner has unexpectedly stopped"}`),
			fmt.Errorf(`ollama returned status 500: {"error":"model runner has unexpectedly stopped"}`),
			fmt.Errorf(`ollama returned status 500: {"error":"model runner has unexpectedly stopped"}`),
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What is the weather?", client, stdout, stderr)
	if err == nil {
		t.Fatal("expected unrecovered LLM error")
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if result.Command != "" || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("command should not execute on LLM failure: result=%#v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionLLMFailureAfterProgressPreservesLastCommandSuccess(t *testing.T) {
	client := &fakeCommandDecisionClient{
		responses: []string{
			`{"command":"printf 'created package.json\n'","done":false,"answer":""}`,
		},
		errors: []error{
			nil,
			context.DeadlineExceeded,
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEvents(
		context.Background(),
		"create the next project step",
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
	)
	if err == nil {
		t.Fatal("expected planner error after progress")
	}
	if result.ExitCode != 0 || !result.PartialProgress {
		t.Fatalf("result should preserve successful command progress: %#v", result)
	}
	if result.Command != "printf 'created package.json\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if !structuredEventsContain(events, "structured_planner_failed_after_progress") {
		t.Fatalf("missing planner-after-progress event: %#v", events)
	}
}

func TestStructuredCommandDecisionRetriesUntilLLMSaysDone(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-first-failed-evidence","done":false,"answer":""}`,
		`{"command":"printf 'second creative attempt\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"second attempt worked"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "Find a working solution.", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want 3", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want 2", result.Observations)
	}
	if result.Observations[0].ExitCode != 2 || result.Observations[1].ExitCode != 0 {
		t.Fatalf("exit codes = %#v", result.Observations)
	}
	if result.Answer != "second attempt worked" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !strings.Contains(client.prompts[1], "omni-nxt-first-failed-evidence") {
		t.Fatalf("second LLM call did not receive first observation: %s", client.prompts[1])
	}
	if stdout.String() != "second creative attempt\n" || !strings.Contains(stderr.String(), "omni-nxt-first-failed-evidence") {
		t.Fatalf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionRejectsDoneAfterOnlyFailedCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-broken-lookup","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"failed, try again later"}`,
		`{"command":"printf 'alternate public source result\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"alternate public source result"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "Find current public information.", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 4 {
		t.Fatalf("llm calls = %d, want 4", client.calls)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want failed command + rejection + successful command", result.Observations)
	}
	if result.Observations[0].ExitCode != 2 {
		t.Fatalf("first command exit = %d, want 2", result.Observations[0].ExitCode)
	}
	if result.Observations[1].Command != "" || !strings.Contains(result.Observations[1].Stderr, "no successful command") {
		t.Fatalf("second observation should reject done after failure: %#v", result.Observations[1])
	}
	if result.Command != "printf 'alternate public source result\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "alternate public source result" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if stdout.String() != "alternate public source result\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "omni-nxt-broken-lookup") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestStructuredCommandDecisionRejectsDoneAfterLatestCommandFailed(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'first source result\n'","done":false,"answer":""}`,
		`{"command":"ls /omni-nxt-second-source-failed","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"first source result"}`,
		`{"command":"printf 'third source result\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"third source result"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "Find current public information.", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 5 {
		t.Fatalf("llm calls = %d, want 5", client.calls)
	}
	if len(result.Observations) != 4 {
		t.Fatalf("observations = %#v, want success + failure + rejection + success", result.Observations)
	}
	if result.Observations[2].Command != "" || !strings.Contains(result.Observations[2].Stderr, "latest real command failed") {
		t.Fatalf("third observation should reject done after latest failure: %#v", result.Observations[2])
	}
	if result.Answer != "third source result" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionCanAskUserAndContinue(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-permission-denied","done":false,"answer":""}`,
		`{"command":"","done":false,"answer":"","ask":true,"question":"Need permission to run sudo install command. Approve?"}`,
		`{"command":"printf 'installed after approval\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"installed after approval"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	asked := []string{}

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Install the required tool if needed.",
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			asked = append(asked, question)
			return "approved", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 {
		t.Fatalf("asked = %#v, want one question", asked)
	}
	if client.calls != 4 {
		t.Fatalf("llm calls = %d, want 4", client.calls)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want failed command + user answer + command", result.Observations)
	}
	if result.Observations[1].Question == "" || result.Observations[1].UserResponse != "approved" {
		t.Fatalf("second observation should carry user response: %#v", result.Observations[1])
	}
	if !strings.Contains(client.prompts[2], `"user_response":"approved"`) {
		t.Fatalf("third prompt missing user response: %s", client.prompts[2])
	}
	if result.Answer != "installed after approval" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionCancelWhileWaitingForUserInputStopsRun(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-before-ask","done":false,"answer":""}`,
		`{"command":"","done":false,"answer":"","ask":true,"question":"Need approval to continue."}`,
		`{"command":"printf 'after cancel should not run\n'","done":false,"answer":""}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Run a command that needs approval.",
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		func(ctx context.Context, question string) (string, error) {
			return "", context.Canceled
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if strings.Contains(stdout.String(), "after cancel should not run") {
		t.Fatalf("command ran after cancel: stdout=%q stderr=%q result=%#v", stdout.String(), stderr.String(), result)
	}
	if !structuredEventsContain(events, "structured_user_input_cancelled") {
		t.Fatalf("events = %#v, want structured_user_input_cancelled", events)
	}
	if got := result.Observations[len(result.Observations)-1]; !strings.Contains(got.Stderr, "user input cancelled") || got.Command != "" {
		t.Fatalf("cancel observation should not dispatch command: %#v", got)
	}
}

func TestStructuredCommandDecisionEmptyApprovalInputDoesNotApproveDependencyInstall(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"approval-test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm install react-router-dom","done":false,"answer":""}`,
		`{"command":"printf 'continued without dependency\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"continued without dependency"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	asks := 0
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"Build a single-page notes app without adding routing.",
		nil,
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			asks++
			return "", nil
		},
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	if err != nil {
		t.Fatal(err)
	}
	if asks != 1 {
		t.Fatalf("asks = %d, want dependency approval ask", asks)
	}
	if strings.Contains(stdout.String(), "react-router-dom") {
		t.Fatalf("empty approval input should not run dependency install: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if result.Answer != "continued without dependency" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionAskWithCommandIsRejectedAndDoesNotExecute(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-blocked-first","done":false,"answer":""}`,
		`{"command":"printf 'ran approved command\n'","done":false,"answer":"","ask":true,"question":"Proceed with creating the requested project directory?"}`,
		`{"command":"printf 'safe followup\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"safe followup"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	asked := []string{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Create the requested project.",
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		func(ctx context.Context, question string) (string, error) {
			asked = append(asked, question)
			return "yes", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 0 {
		t.Fatalf("mixed ask+command should not ask user: %#v", asked)
	}
	if strings.Contains(stdout.String(), "ran approved command") {
		t.Fatalf("mixed ask+command executed unexpectedly: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "safe followup") {
		t.Fatalf("safe followup did not run: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !structuredEventsContain(events, "structured_payload_rejected_mixed_ask_command") {
		t.Fatalf("events = %#v, want structured_payload_rejected_mixed_ask_command", events)
	}
	if result.Observations[1].RejectedCommand == "" || !strings.Contains(result.Observations[1].Stderr, "ask=true cannot be combined") {
		t.Fatalf("mixed ask+command should be rejected observation: %#v", result.Observations)
	}
}

func TestStructuredCommandDecisionRejectsMalformedAskWhenCommandIsPresent(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'weather evidence\n'","done":false,"answer":"","ask":true,"question":""}`,
		`{"command":"printf 'weather evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"weather evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Check the requested weather.",
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		func(ctx context.Context, question string) (string, error) {
			t.Fatalf("ask callback should not run for empty question with executable command: %q", question)
			return "", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(stdout.String(), "weather evidence") != 1 {
		t.Fatalf("mixed ask command should be rejected before later safe command: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(result.Command, "weather evidence") {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "weather evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !structuredEventsContain(events, "structured_payload_rejected_mixed_ask_command") {
		t.Fatalf("events = %#v, want structured_payload_rejected_mixed_ask_command", events)
	}
}

func TestStructuredCommandDecisionRepeatedApprovalQuestionWithCommandIsRejected(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"ls /omni-nxt-blocked-first","done":false,"answer":""}`,
		`{"command":"","done":false,"answer":"","ask":true,"question":"Proceed with creating the requested project directory?"}`,
		`{"command":"printf 'created after reused approval\n'","done":false,"answer":"","ask":true,"question":"Proceed with creating the requested project directory?"}`,
		`{"command":"printf 'created after correction\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"created after correction"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	askCount := 0

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Create the requested project.",
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			askCount++
			return "yes", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if askCount != 1 {
		t.Fatalf("askCount = %d, want repeated question reused", askCount)
	}
	if strings.Contains(stdout.String(), "created after reused approval") {
		t.Fatalf("mixed ask+command executed unexpectedly: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if result.Answer != "created after correction" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandObservationsKeepCommandOutputIsolated(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "printf 'npm install output\n'", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if err := runStructuredPayloadCommand(context.Background(), 2, "find . -maxdepth 1 -type f", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) < 2 {
		t.Fatalf("observations = %#v", result.Observations)
	}
	installObs := result.Observations[0]
	findObs := result.Observations[1]
	if installObs.CommandID == "" || findObs.CommandID == "" || installObs.CommandID == findObs.CommandID {
		t.Fatalf("command ids not unique/stable: install=%#v find=%#v", installObs, findObs)
	}
	if !strings.Contains(installObs.Stdout, "npm install output") {
		t.Fatalf("install output missing from first observation: %#v", installObs)
	}
	if strings.Contains(findObs.Stdout, "npm install output") {
		t.Fatalf("find observation contains prior command output: %#v", findObs)
	}
	if !strings.Contains(findObs.Stdout, "package.json") {
		t.Fatalf("find observation missing find output: %#v", findObs)
	}
}
