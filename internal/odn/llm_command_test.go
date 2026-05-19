package odn

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

type fakeCommandDecisionClient struct {
	responses []string
	errors    []error
	calls     int
	prompts   []string
	requests  []OllamaChatRequest
}

func (f *fakeCommandDecisionClient) ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	f.calls++
	f.requests = append(f.requests, req)
	if len(req.Messages) > 0 {
		f.prompts = append(f.prompts, req.Messages[len(req.Messages)-1].Content)
	}
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return OllamaChatResponse{}, err
		}
	}
	if len(f.responses) == 0 {
		return OllamaChatResponse{Content: `{"command":"","done":true,"answer":"done"}`}, nil
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return OllamaChatResponse{Content: response}, nil
}

func TestStructuredCommandDecisionAlwaysCallsLLMForNaturalLanguagePrompts(t *testing.T) {
	tests := []struct {
		prompt  string
		command string
		want    string
	}{
		{
			prompt:  "Where am I in the filesystem?",
			command: "printf 'filesystem-result\n'",
			want:    "filesystem-result\n",
		},
		{
			prompt:  "What is the current calendar timestamp?",
			command: "printf 'timestamp-result\n'",
			want:    "timestamp-result\n",
		},
		{
			prompt:  "Which account is running this process?",
			command: "printf 'account-result\n'",
			want:    "account-result\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.prompt, func(t *testing.T) {
			client := &fakeCommandDecisionClient{responses: []string{
				`{"command":` + quoteJSONForTest(tc.command) + `,"done":false,"answer":""}`,
				`{"command":"","done":true,"answer":"done"}`,
			}}
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			result, err := RunStructuredCommandDecision(context.Background(), tc.prompt, client, stdout, stderr)
			if err != nil {
				t.Fatal(err)
			}
			if client.calls != 2 {
				t.Fatalf("llm calls = %d, want 2", client.calls)
			}
			if len(client.prompts) != 2 {
				t.Fatalf("llm prompts = %#v, want 2 calls", client.prompts)
			}
			if !strings.Contains(client.prompts[0], quoteJSONForTest(tc.prompt)) {
				t.Fatalf("first llm prompt = %q, want original prompt encoded", client.prompts[0])
			}
			if client.requests[0].ContextSystem == "" {
				t.Fatal("structured command request should place planner contract in context system")
			}
			if len(client.requests[0].Messages) != 1 || client.requests[0].Messages[0].Role != "user" {
				t.Fatalf("structured command request should isolate current payload as one user message: %#v", client.requests[0].Messages)
			}
			if result.Command != tc.command {
				t.Fatalf("command = %q, want %q", result.Command, tc.command)
			}
			if stdout.String() != tc.want {
				t.Fatalf("stdout = %q, want %q; stderr=%q", stdout.String(), tc.want, stderr.String())
			}
		})
	}
}

func TestStructuredCommandRequestIsolatesCurrentPromptFromHistory(t *testing.T) {
	req := buildStructuredCommandRequest(
		"Yes, but will it rain though was my question",
		[]Message{
			{Role: "user", Content: "what's the weather in Pattaya right now?"},
			{Role: "assistant", Content: "The weather in Pattaya, Thailand today is Partly Cloudy with temperatures ranging from +33C to +41C."},
		},
		nil,
	)
	if req.ContextSystem == "" {
		t.Fatal("missing context system")
	}
	if !strings.Contains(req.ContextSystem, "active_task.current_prompt field is the command objective") {
		t.Fatalf("context system missing prompt isolation rule: %s", req.ContextSystem)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages = %#v, want reference history, acknowledgement, active task", req.Messages)
	}
	if !strings.Contains(req.Messages[0].Content, "reference_history") || !strings.Contains(req.Messages[0].Content, "Pattaya") {
		t.Fatalf("first message missing reference history: %#v", req.Messages)
	}
	content := req.Messages[2].Content
	if !strings.Contains(content, `"active_prompt_open":"Yes, but will it rain though was my question"`) {
		t.Fatalf("payload missing opening active prompt anchor: %s", content)
	}
	if !strings.Contains(content, `"current_prompt":"Yes, but will it rain though was my question"`) {
		t.Fatalf("payload missing authoritative current_prompt: %s", content)
	}
	if !strings.Contains(content, `"active_prompt_close":"Yes, but will it rain though was my question"`) {
		t.Fatalf("payload missing closing active prompt anchor: %s", content)
	}
	if strings.Contains(content, "Pattaya") || strings.Contains(content, "reference_history") {
		t.Fatalf("active task payload should not contain reference history: %s", content)
	}
}

func TestStructuredCommandRequestUsesTerseInertMemoryRecords(t *testing.T) {
	req := buildStructuredCommandRequest(
		"What time is it in Virginia right now?",
		[]Message{
			{Role: "user", Content: "What's the weather in Pattaya right now?"},
			{Role: "assistant", Content: "Command: curl -s wttr.in/Pattaya+Thailand?format=%C+%t+%f\nAnswer: Partly cloudy +33C +41C."},
			{Role: "user", Content: "Build a demo Go project in ~/Projects/tmp-project."},
			{Role: "assistant", Content: "Asked for permission to create the requested project directory."},
		},
		nil,
	)
	if len(req.Messages) != 3 {
		t.Fatalf("messages = %#v, want separated memory and active task", req.Messages)
	}
	history := req.Messages[0].Content
	for _, want := range []string{`"reference_history"`, `"not_prompt":true`, `"memory_style":"terse_reference_only"`, `"memory_note"`} {
		if !strings.Contains(history, want) {
			t.Fatalf("history missing %q: %s", want, history)
		}
	}
	active := req.Messages[2].Content
	if strings.Contains(active, "Pattaya") || strings.Contains(active, "tmp-project") || strings.Contains(active, "wttr.in") {
		t.Fatalf("active prompt is polluted by memory: %s", active)
	}
	if strings.Count(active, "What time is it in Virginia right now?") != 4 {
		t.Fatalf("active prompt should appear as open/current/prompt/close anchors: %s", active)
	}
}

func TestStructuredCommandDecisionAnswersActivePromptDespiteConflictingMemory(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'Virginia time evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Virginia time evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	history := []Message{
		{Role: "user", Content: "What's the weather in Pattaya right now?"},
		{Role: "assistant", Content: "Command: curl -s wttr.in/Pattaya+Thailand?format=%C+%t+%f\nAnswer: Partly cloudy +33C +41C."},
		{Role: "user", Content: "What are the current events in Saipan?"},
		{Role: "assistant", Content: "Command: curl -s https://news.google.com/rss/search?q=Saipan"},
		{Role: "user", Content: "Build a React TypeScript app."},
		{Role: "assistant", Content: "Command: npm run build"},
	}

	result, err := RunStructuredCommandDecisionWithHistoryEventsAndAsk(
		context.Background(),
		"What time is it in Virginia right now?",
		history,
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			t.Fatalf("should not ask when active prompt is specific: %q", question)
			return "", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "printf 'Virginia time evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if stdout.String() != "Virginia time evidence\n" || result.Answer != "Virginia time evidence" {
		t.Fatalf("unexpected result stdout=%q answer=%q", stdout.String(), result.Answer)
	}
	if len(client.requests[0].Messages) != 3 {
		t.Fatalf("messages = %#v, want memory + ack + active task", client.requests[0].Messages)
	}
	active := client.requests[0].Messages[2].Content
	for _, polluted := range []string{"Pattaya", "Saipan", "React", "wttr.in", "news.google.com", "npm run build"} {
		if strings.Contains(active, polluted) {
			t.Fatalf("active task contains memory %q: %s", polluted, active)
		}
	}
	if strings.Count(active, "What time is it in Virginia right now?") != 4 {
		t.Fatalf("active prompt not anchored open/current/prompt/close: %s", active)
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
}

func TestClassifyStructuredLLMFailureIdentifiesRunnerCrash(t *testing.T) {
	err := fmt.Errorf(`ollama returned status 500: {"error":"model runner has unexpectedly stopped"}`)
	if got := classifyStructuredLLMFailure(err); got != "ollama_model_runner_crash_or_restart" {
		t.Fatalf("diagnosis = %q", got)
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

func TestStructuredCommandDecisionRetriesUntilLLMSaysDone(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'first failed evidence\n' && exit 7","done":false,"answer":""}`,
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
	if result.Observations[0].ExitCode != 7 || result.Observations[1].ExitCode != 0 {
		t.Fatalf("exit codes = %#v", result.Observations)
	}
	if result.Answer != "second attempt worked" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !strings.Contains(client.prompts[1], "first failed evidence") {
		t.Fatalf("second LLM call did not receive first observation: %s", client.prompts[1])
	}
	if stdout.String() != "first failed evidence\nsecond creative attempt\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStructuredCommandDecisionRejectsDoneAfterOnlyFailedCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'broken lookup\n' >&2; exit 2","done":false,"answer":""}`,
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
	if stderr.String() != "broken lookup\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestStructuredCommandDecisionRejectsDoneAfterLatestCommandFailed(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'first source result\n'","done":false,"answer":""}`,
		`{"command":"printf 'second source failed\n' >&2; exit 2","done":false,"answer":""}`,
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
		`{"command":"printf 'permission denied\n' >&2; exit 1","done":false,"answer":""}`,
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

func TestStructuredCommandDecisionAskWithCommandRunsAfterApproval(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'blocked first\n' >&2; exit 1","done":false,"answer":""}`,
		`{"command":"printf 'ran approved command\n'","done":false,"answer":"","ask":true,"question":"Proceed with creating the requested project directory?"}`,
		`{"command":"","done":true,"answer":"ran approved command"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	asked := []string{}

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(
		context.Background(),
		"Create the requested project.",
		client,
		stdout,
		stderr,
		nil,
		func(ctx context.Context, question string) (string, error) {
			asked = append(asked, question)
			return "yes", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 {
		t.Fatalf("asked = %#v, want one approval", asked)
	}
	if !strings.Contains(stdout.String(), "ran approved command") {
		t.Fatalf("approval-gated command did not run: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want failed command + user answer + approved command", result.Observations)
	}
	if result.Observations[1].Question == "" || result.Observations[2].Command == "" {
		t.Fatalf("expected user answer followed by command observation: %#v", result.Observations)
	}
}

func TestStructuredCommandDecisionIgnoresMalformedAskWhenCommandIsPresent(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'weather evidence\n'","done":false,"answer":"","ask":true,"question":""}`,
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
	if !strings.Contains(stdout.String(), "weather evidence") {
		t.Fatalf("command did not run: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(result.Command, "weather evidence") {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "weather evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !structuredEventsContain(events, "structured_ask_ignored") {
		t.Fatalf("events = %#v, want structured_ask_ignored", events)
	}
}

func TestStructuredCommandDecisionReusesRepeatedApprovalQuestionWithCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'blocked first\n' >&2; exit 1","done":false,"answer":""}`,
		`{"command":"","done":false,"answer":"","ask":true,"question":"Proceed with creating the requested project directory?"}`,
		`{"command":"printf 'created after reused approval\n'","done":false,"answer":"","ask":true,"question":"Proceed with creating the requested project directory?"}`,
		`{"command":"","done":true,"answer":"created after reused approval"}`,
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
	if !strings.Contains(stdout.String(), "created after reused approval") {
		t.Fatalf("reused approval command did not run: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if result.Answer != "created after reused approval" {
		t.Fatalf("answer = %q", result.Answer)
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
		`{"command":"printf 'needs sudo\n' >&2; exit 1","done":false,"answer":""}`,
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
	if stdout.Len() != 0 || stderr.String() != "needs sudo\n" {
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
		"structured_llm_request_started",
		"structured_llm_payload_received",
		"structured_command_started",
		"structured_command_finished",
		"structured_llm_request_started",
		"structured_llm_payload_received",
		"structured_done_accepted",
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

func TestStructuredCommandDecisionExhaustsRepeatedDoneWithNonzeroFailure(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
		`{"command":"","done":true,"answer":"done without evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "create a requested filesystem state", client, stdout, stderr)
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if _, ok := err.(CommandDecisionExhaustedError); !ok {
		t.Fatalf("err = %T %v, want CommandDecisionExhaustedError", err, err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exhausted result exit code = 0, want nonzero")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected command output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStructuredCommandDecisionRejectsEmptyCommandAndContinues(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":""}`,
		`{"command":"printf 'searched evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"searched evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "you have access to the internet and can search", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want 3", client.calls)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejection + command", result.Observations)
	}
	if result.Observations[0].Command != "" || !strings.Contains(result.Observations[0].Stderr, "empty command") {
		t.Fatalf("first observation should reject empty command: %#v", result.Observations[0])
	}
	if result.Command != "printf 'searched evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if stdout.String() != "searched evidence\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStructuredCommandDecisionPromptForbidsPlaceholderCredentials(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'ok\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"ok"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	_, err := RunStructuredCommandDecision(context.Background(), "Get external current data.", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) < 1 || len(client.requests[0].Messages) == 0 {
		t.Fatalf("missing captured LLM request: %#v", client.requests)
	}
	systemPrompt := client.requests[0].ContextSystem
	for _, want := range []string{
		"Do not use placeholder credentials.",
		"Do not call APIs that require unavailable keys.",
		"Never put placeholder key text in a command.",
		"To ask the user for needed help, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"ask\":true,\"question\":\"brief specific question\"}.",
		"If must_return_command is true, done=true is invalid; return a non-empty command.",
		"If must_return_command is true, ask=true is invalid; inspect or try a command first.",
		"If the latest real command succeeded, ask=true is invalid; continue, verify, or finish from evidence.",
		"Do not return done=true until at least one command has exit_code 0.",
		"If the latest command failed, return a different command instead of done=true.",
		"Use shell commands to satisfy requests; do not answer from memory when command evidence is required.",
		"Never return an empty command when done=false.",
		"If a command fails, the failure is recorded in observations; use that context to pivot to a different command, source, or tool.",
		"Ask the user only when progress requires permission, credentials, sudo, destructive approval, or a choice that cannot be inferred from evidence.",
		"Do not ask for help when another non-destructive command, public source, or local inspection can be tried.",
		"Available terminal tools may include bash, curl, python3, sed, awk, grep, jq, date, uname, and package managers; discover with commands when uncertain.",
		"Each command runs in a fresh shell; cd does not persist to the next step.",
		"Use absolute paths or include cd in the same command that needs it.",
		"Use current_working_directory for project creation unless the user explicitly provided another path.",
		"Do not create demo projects in the home directory unless the user explicitly asked for home.",
		"To identify the operating system, inspect command evidence such as uname and /etc/os-release.",
		"For identification tasks, inspect available package managers only; do not ask for permission to proceed with a package manager.",
		"Before OS-specific package or install advice, verify OS, distro, version, architecture, and available package managers with commands.",
		"If a needed tool is missing, identify install options from verified OS/package-manager evidence.",
		"Do not install missing tools unless the user explicitly asked to install or approved installation.",
		"When installation is not approved, answer with the proposed install command and ask for approval.",
		"For external facts, use public unauthenticated sources.",
		"For timely public information, use internet commands by default.",
		"For current, recent, latest, today, or now public facts, the first accepted command should gather live evidence from the internet.",
		"For filesystem changes, run shell commands that create or modify the requested filesystem state.",
		"For local static web app demos, create files locally and serve them with a local server such as python3 http.server.",
		"For Go CLI demos, use curl to discover the latest Go release from go.dev/dl/?mode=json, install that Go toolchain into a user-writable project directory unless system installation is approved, then build, test, and run the app.",
		"The Go release JSON has version and files[].filename fields; construct downloads as https://go.dev/dl/<filename>.",
		"For Go CLI demos, do not return done=true until go test, go build, and the built executable have all succeeded.",
		"Do not treat null or empty JSON query output as useful evidence.",
		"For npm React TypeScript demos, prefer a minimal Vite project with package.json and src files; do not use create-react-app.",
		"For npm install/build commands in tests, keep output concise when possible.",
		"When starting a background server, use nohup or equivalent and write the background process PID with $! if a PID file is requested.",
		"When starting a background server, redirect stdout and stderr away from the command pipe.",
		"Do not background file creation or setup commands; only background the long-running server process.",
		"When chaining commands before a background server, use semicolons before nohup; avoid '&& nohup ... &' because bash may background the setup chain.",
		"After starting a local server, verify it with a short curl retry loop before done=true.",
		"Do not ask for public sources when the task can be completed with local files.",
		"If output reports invalid credentials, try a no-key public source before done.",
		"If the shell reports a syntax or quoting error, correct the command or use a simpler command.",
		"Match the command source to the requested fact type.",
		"Public no-key internet sources available: wttr.in, news.google.com/rss/search?q=<query>, duckduckgo.com/html/?q=<query>.",
		"Prefer simple curl commands that print readable evidence over fragile HTML parsing.",
		"For current time, prefer shell time/date commands or public no-key time sources.",
		"For location-specific time, produce local-time evidence for that location; do not answer from UTC unless UTC was requested.",
		"Do not use weather services as time sources.",
		"If using shell date for a location, choose an IANA timezone and prefix the command with TZ=Area/City before date.",
		"Do not pass TZ=Area/City as an argument to date.",
		"Prefer concise command output; use format/query options instead of large pages when available.",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
}

func TestStructuredCommandDecisionUserMessageCarriesCommandRequirementState(t *testing.T) {
	message := buildStructuredCommandUserMessage("make a project", nil)
	if !strings.Contains(message, `"must_return_command":true`) {
		t.Fatalf("message missing must_return_command=true: %s", message)
	}
	if !strings.Contains(message, `"real_command_observation_count":0`) {
		t.Fatalf("message missing real command count: %s", message)
	}
	if !strings.Contains(message, `"current_working_directory":`) {
		t.Fatalf("message missing current working directory: %s", message)
	}
	if !strings.Contains(message, `"successful_command_count":0`) || !strings.Contains(message, `"failed_command_count":0`) {
		t.Fatalf("message missing command outcome counts: %s", message)
	}

	message = buildStructuredCommandUserMessage("make a project", []StructuredCommandObservation{{
		Step:     1,
		Command:  "mkdir -p /tmp/example",
		ExitCode: 0,
	}})
	if !strings.Contains(message, `"must_return_command":false`) {
		t.Fatalf("message missing must_return_command=false: %s", message)
	}
	if !strings.Contains(message, `"real_command_observation_count":1`) {
		t.Fatalf("message missing real command count after command: %s", message)
	}
	if !strings.Contains(message, `"successful_command_count":1`) || !strings.Contains(message, `"failed_command_count":0`) {
		t.Fatalf("message missing successful command count after command: %s", message)
	}
}

func TestStructuredCommandDecisionFirstRequestSchemaRequiresCommand(t *testing.T) {
	format := buildStructuredCommandResponseFormat(nil)
	props := format["properties"].(map[string]interface{})
	command := props["command"].(map[string]interface{})
	done := props["done"].(map[string]interface{})
	if command["minLength"] != 1 {
		t.Fatalf("first command schema missing minLength: %#v", command)
	}
	if enum, ok := done["enum"].([]bool); !ok || len(enum) != 1 || enum[0] {
		t.Fatalf("first command schema should force done=false: %#v", done)
	}

	format = buildStructuredCommandResponseFormat([]StructuredCommandObservation{{Command: "printf ok", ExitCode: 0}})
	props = format["properties"].(map[string]interface{})
	command = props["command"].(map[string]interface{})
	done = props["done"].(map[string]interface{})
	if _, ok := command["minLength"]; ok {
		t.Fatalf("post-evidence command schema should allow empty done command: %#v", command)
	}
	if _, ok := done["enum"]; ok {
		t.Fatalf("post-evidence done schema should allow true/false: %#v", done)
	}
}

func TestStructuredCommandDecisionTruncatesObservationBeforeNextLLMCall(t *testing.T) {
	longOutput := strings.Repeat("x", defaultStructuredObservationChars+500)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForTest("printf '"+longOutput+"'") + `,"done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "produce long output", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %#v", result.Observations)
	}
	if len(result.Observations[0].Stdout) > defaultStructuredObservationChars+20 {
		t.Fatalf("observation was not truncated: len=%d", len(result.Observations[0].Stdout))
	}
	if !strings.Contains(result.Observations[0].Stdout, "[truncated]") {
		t.Fatalf("truncated marker missing: %q", result.Observations[0].Stdout)
	}
	if len(stdout.String()) != len(longOutput) {
		t.Fatalf("user stdout should keep full output: got len=%d want=%d", len(stdout.String()), len(longOutput))
	}
}

func TestExecuteStructuredCommandKillsBackgroundPipeHolderOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	exitCode, err := ExecuteStructuredCommand(ctx, "sleep 60 &", &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if exitCode == 0 {
		t.Fatalf("exit code = 0, want nonzero")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("command did not stop promptly after context cancellation")
	}
}

func TestCommandDecisionSourceAuditNoPromptPhraseMatching(t *testing.T) {
	sourcePath := filepath.Join("llm_command.go")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	forbidden := []string{
		"strings.Contains(prompt",
		"strings.Contains(strings.ToLower(prompt",
		"strings.HasPrefix(prompt",
		"strings.HasSuffix(prompt",
		"regexp.",
		"MatchString(prompt",
		"switch prompt",
		"case \"Where am I",
		"case \"What is the current",
		"case \"Which account",
	}
	for _, needle := range forbidden {
		if strings.Contains(source, needle) {
			t.Fatalf("command decision source contains forbidden prompt phrase matching %q", needle)
		}
	}
}

func quoteJSONForTest(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(value) + `"`
}

func structuredEventsContain(events []StructuredCommandEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
