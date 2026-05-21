package omni

import (
	"bytes"
	"context"
	"encoding/json"
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

type fakeStructuredResponseEvaluator struct {
	evaluations []StructuredLLMEvaluation
	errors      []error
	inputs      []StructuredLLMEvaluationInput
}

func (f *fakeStructuredResponseEvaluator) EvaluateStructuredLLMResponse(ctx context.Context, input StructuredLLMEvaluationInput) (StructuredLLMEvaluation, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return StructuredLLMEvaluation{}, err
		}
	}
	if len(f.evaluations) == 0 {
		return StructuredLLMEvaluation{Confidence: 100, Feedback: ""}, nil
	}
	evaluation := f.evaluations[0]
	f.evaluations = f.evaluations[1:]
	return evaluation, nil
}

type fakeShellCommandSpecialist struct {
	proposals []ShellCommandProposal
	errors    []error
	inputs    []ShellCommandSpecialistInput
}

type fakePromptInterpreter struct {
	interpretations []PromptInterpretation
	errors          []error
	inputs          []PromptInterpretationInput
}

func (f *fakePromptInterpreter) InterpretPrompt(ctx context.Context, input PromptInterpretationInput) (PromptInterpretation, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return PromptInterpretation{}, err
		}
	}
	if len(f.interpretations) == 0 {
		return PromptInterpretation{}, nil
	}
	interpretation := f.interpretations[0]
	f.interpretations = f.interpretations[1:]
	return interpretation, nil
}

type fakeContextSummarizer struct {
	contexts []MinimalContext
	errors   []error
	inputs   []MinimalContextInput
}

type fakeCompletionChecker struct {
	checks []CompletionCheck
	errors []error
	inputs []CompletionCheckInput
}

func (f *fakeCompletionChecker) CheckCompletion(ctx context.Context, input CompletionCheckInput) (CompletionCheck, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return CompletionCheck{}, err
		}
	}
	if len(f.checks) == 0 {
		return CompletionCheck{}, nil
	}
	check := f.checks[0]
	f.checks = f.checks[1:]
	return check, nil
}

func (f *fakeContextSummarizer) SummarizeContext(ctx context.Context, input MinimalContextInput) (MinimalContext, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return MinimalContext{}, err
		}
	}
	if len(f.contexts) == 0 {
		return MinimalContext{}, nil
	}
	context := f.contexts[0]
	f.contexts = f.contexts[1:]
	return context, nil
}

func (f *fakeShellCommandSpecialist) ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return ShellCommandProposal{}, err
		}
	}
	if len(f.proposals) == 0 {
		return ShellCommandProposal{Command: "printf 'default shell evidence\n'", Rationale: "default"}, nil
	}
	proposal := f.proposals[0]
	f.proposals = f.proposals[1:]
	return proposal, nil
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
	activeTask := activeTaskJSONForTest(t, active)
	if strings.Contains(activeTask, "Pattaya") || strings.Contains(activeTask, "tmp-project") || strings.Contains(activeTask, "wttr.in") {
		t.Fatalf("active task is polluted by memory: %s", activeTask)
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
	if len(client.requests[0].Messages) != 1 {
		t.Fatalf("messages = %#v, want active task only without interpreter-approved history", client.requests[0].Messages)
	}
	active := client.requests[0].Messages[0].Content
	activeTask := activeTaskJSONForTest(t, active)
	for _, polluted := range []string{"Pattaya", "Saipan", "React", "wttr.in", "news.google.com", "npm run build"} {
		if strings.Contains(activeTask, polluted) {
			t.Fatalf("active task contains memory %q: %s", polluted, activeTask)
		}
	}
	if strings.Count(active, "What time is it in Virginia right now?") != 4 {
		t.Fatalf("active prompt not anchored open/current/prompt/close: %s", active)
	}
}

func TestStructuredCommandDecisionDoesNotSendReferenceHistoryForStandalonePrompt(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'react project only\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"react project only"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RequiresReferenceHistory: false,
	}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary:   "Create only the requested React project.",
		OpenItems: []string{"React project"},
	}}}
	history := []Message{
		{Role: "user", Content: "Create a Stimulus Tailwind RecyclrJS webpack calculator."},
		{Role: "assistant", Content: "Installed @hotwired/stimulus tailwindcss recyclr-js webpack."},
	}

	_, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"Create a new React project.",
		history,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		nil,
		structuredCommandDecisionRunConfig{
			PromptInterpreter: interpreter,
			ContextSummarizer: summarizer,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(summarizer.inputs) != 1 {
		t.Fatalf("summarizer inputs = %d", len(summarizer.inputs))
	}
	if len(summarizer.inputs[0].History) != 0 {
		t.Fatalf("standalone prompt leaked history to summarizer: %#v", summarizer.inputs[0].History)
	}
	firstRequest := joinOllamaMessageContent(client.requests[0].Messages)
	for _, polluted := range []string{"Stimulus", "Tailwind", "RecyclrJS", "webpack", "@hotwired/stimulus", "recyclr-js"} {
		if strings.Contains(firstRequest, polluted) {
			t.Fatalf("standalone planner request contains prior project dependency %q: %s", polluted, firstRequest)
		}
	}
}

func TestStructuredDependencyScopeRejectsMemorySuggestedPackages(t *testing.T) {
	workspace := t.TempDir()
	ledger := []StructuredObjective{
		{ID: "react_project", Description: "Create a React project", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true, Packages: []string{"react", "react-dom", "vite"}},
		{ID: "usual_frontend_stack", Description: "User likes Tailwind, RecyclrJS, and Stimulus", Status: "pending", Source: structuredObjectiveSourceMemorySuggested, Packages: []string{"tailwindcss", "recyclrjs", "@hotwired/stimulus"}},
	}
	err := validateStructuredCommandForRun("npm install react react-dom vite tailwindcss recyclrjs @hotwired/stimulus", nil, workspace, ledger)
	if err == nil {
		t.Fatal("expected memory-suggested dependencies to be rejected")
	}
	for _, want := range []string{"tailwindcss", "recyclrjs", "@hotwired/stimulus"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if memory := structuredCapabilityMemoryForRejectedResponse("npm install react react-dom vite tailwindcss recyclrjs", err.Error()); memory != structuredScopeCapabilityMemory {
		t.Fatalf("scope capability memory = %q", memory)
	}
}

func TestStructuredCommandDecisionRejectsShellSpecialistScopeDrift(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"install dependencies for the React project"}`,
		`{"command":"touch package.json","done":false,"answer":"","objective_ledger":[{"id":"react_project","description":"Create a React project","status":"satisfied","source":"user_explicit","required":true,"packages":["react","react-dom","vite"],"evidence":"package.json created"}]}`,
		`{"command":"test -f package.json && ls package.json","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"React project started"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "react_project", Description: "Create a React project", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true, Packages: []string{"react", "react-dom", "vite"}},
		},
	}}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "npm install react react-dom vite tailwindcss recyclrjs",
		Rationale: "install proposed dependencies",
	}}}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"create a new React project",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
			ShellSpecialist:         shell,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) == 0 || result.Observations[0].RejectedCommand == "" {
		t.Fatalf("expected rejected shell specialist command: %#v", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stderr, "dependency scope drift") {
		t.Fatalf("expected scope drift rejection: %#v", result.Observations[0])
	}
	if result.Observations[0].CapabilityMemory != structuredScopeCapabilityMemory {
		t.Fatalf("capability memory = %q", result.Observations[0].CapabilityMemory)
	}
}

func TestStructuredCommandDecisionRejectsPlannerScopeDriftDependencyCommand(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm install react react-dom vite tailwindcss recyclrjs @hotwired/stimulus","done":false,"answer":""}`,
		`{"command":"touch package.json","done":false,"answer":"","objective_ledger":[{"id":"react_project","description":"Create a React project","status":"satisfied","source":"user_explicit","required":true,"packages":["react","react-dom","vite"],"evidence":"package.json created"}]}`,
		`{"command":"test -f package.json && ls package.json","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"React project started"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "react_project", Description: "Create a React project", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true, Packages: []string{"react", "react-dom", "vite"}},
			{ID: "usual_stack", Description: "User likes Tailwind, RecyclrJS, and Stimulus", Status: "pending", Source: structuredObjectiveSourceMemorySuggested, Required: false, Packages: []string{"tailwindcss", "recyclrjs", "@hotwired/stimulus"}},
		},
	}}}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"create a new React project",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) == 0 || result.Observations[0].RejectedCommand == "" {
		t.Fatalf("expected rejected planner command: %#v", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stderr, "tailwindcss") || !strings.Contains(result.Observations[0].Stderr, "recyclrjs") {
		t.Fatalf("scope rejection missing packages: %#v", result.Observations[0])
	}
}

func TestStructuredCommandDecisionEvaluatorScopeDriftBlocksExecutionAtThreshold(t *testing.T) {
	workspace := createReactFixture(t)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"cd /home/gryph/Projects/tmp && npx create-react-app calculator-app","done":false,"answer":""}`,
		`{"command":"printf 'modified existing app\n'","done":false,"answer":""}`,
		`{"command":"pwd","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"modified existing app"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		UserOperation: userOperationModifyExisting,
		ObjectiveLedger: []StructuredObjective{
			{ID: "create_new_react_project", Description: "Create a new React project", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Confidence: 70, Feedback: "The response provides a step and shell command to create a new React project, but it does not align with the user's request for making an existing app into a calculator."},
		{Verdict: "accept", Confidence: 100, Feedback: "on track"},
		{Verdict: "accept", Confidence: 100, Feedback: "on track"},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "existing app modification was observed",
		ObjectiveLedger: []StructuredObjective{
			{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "satisfied", Evidence: "modified existing app"},
		},
	}}}
	var stdout strings.Builder
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "make this existing React app into a calculator", nil, client, &stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
		Evaluator:               evaluator,
		EvaluatorThreshold:      70,
		CompletionChecker:       checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "create-react-app") {
		t.Fatalf("scaffold command appears to have executed: %q", stdout.String())
	}
	if len(result.Observations) == 0 || !strings.Contains(result.Observations[0].Stderr, "scope_drift") {
		t.Fatalf("missing hard scope drift observation: %#v", result.Observations)
	}
	if containsStructuredObjectiveID(result.ObjectiveLedger, "create_new_react_project") {
		t.Fatalf("create-new objective should be filtered for modify-existing task: %#v", result.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionEvaluatorRejectConfidenceCannotOverride(t *testing.T) {
	workspace := createReactFixture(t)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'bad should not run\n'","done":false,"answer":""}`,
		`{"command":"printf 'corrected\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"corrected"}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "reject", Confidence: 100, Feedback: "scope drift"},
		{Verdict: "accept", Confidence: 100, Feedback: "on track"},
		{Verdict: "accept", Confidence: 100, Feedback: "on track"},
	}}
	var stdout strings.Builder
	_, err := runStructuredCommandDecisionWithConfig(context.Background(), "modify existing app", nil, client, &stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		Evaluator:               evaluator,
		EvaluatorThreshold:      70,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "bad should not run") {
		t.Fatalf("reject verdict executed despite confidence 100: %q", stdout.String())
	}
}

func TestStructuredCommandDecisionRepeatedEvaluatorReviseBypassesEvaluatorLoop(t *testing.T) {
	workspace := createReactFixture(t)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'first candidate\n'","done":false,"answer":""}`,
		`{"command":"printf 'accepted after evaluator loop\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"accepted after evaluator loop"}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "revise", Confidence: 100, Feedback: "install Tailwind manually"},
		{Verdict: "revise", Confidence: 100, Feedback: "install Tailwind manually"},
	}}
	var stdout strings.Builder
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue making the app a calculator", nil, client, &stdout, &strings.Builder{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		Evaluator:               evaluator,
		EvaluatorThreshold:      70,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "accepted after evaluator loop") {
		t.Fatalf("second planner command did not execute after evaluator loop bypass: %q", stdout.String())
	}
	if !structuredEventsContain(events, "structured_evaluator_loop_bypassed") {
		t.Fatalf("missing evaluator loop bypass event: %#v", events)
	}
	if result.Answer != "accepted after evaluator loop" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestFormatStructuredCommandChatResponseSummarizesExhaustionBlocker(t *testing.T) {
	result := CommandDecisionResult{
		ExitCode:        1,
		PartialProgress: true,
		ObjectiveLedger: []StructuredObjective{{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending", Required: true, Source: structuredObjectiveSourceUserExplicit}},
		Observations: []StructuredCommandObservation{{
			Step:     40,
			ExitCode: 1,
			Stderr:   "anti_loop: evaluator repeated the same revise feedback",
		}},
	}
	response := formatStructuredCommandChatResponse(result, "", "", "structured command loop exhausted after 40 step(s) without accepted completion")
	for _, want := range []string{"Command: (none accepted)", "Pending objectives: implement_calculator_logic", "Loop blocker: anti_loop", "Stopped: structured command loop exhausted"} {
		if !strings.Contains(response, want) {
			t.Fatalf("response missing %q:\n%s", want, response)
		}
	}
}

func TestStructuredCommandDecisionRecordsElapsedTime(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'done\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
	}}
	result, err := RunStructuredCommandDecision(context.Background(), "produce elapsed metadata", client, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result.StartedAt.IsZero() || result.FinishedAt.IsZero() || result.Elapsed <= 0 {
		t.Fatalf("missing elapsed metadata: started=%v finished=%v elapsed=%v", result.StartedAt, result.FinishedAt, result.Elapsed)
	}
}

func TestValidateStructuredScaffoldScopeBlocksCreateReactAppInModifyMode(t *testing.T) {
	survey := WorksiteSurvey{UserOperation: userOperationModifyExisting, ProjectState: projectStateExistingReactApp}
	err := validateStructuredCommandForRunWithSurvey("npx create-react-app calculator-app", nil, t.TempDir(), nil, survey)
	if err == nil || !strings.Contains(err.Error(), "scope_drift") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateStructuredScaffoldScopeBlocksNpmCreateViteInModifyMode(t *testing.T) {
	survey := WorksiteSurvey{UserOperation: userOperationModifyExisting, ProjectState: projectStateExistingReactApp}
	err := validateStructuredCommandForRunWithSurvey("npm create vite@latest calculator-app -- --template react", nil, t.TempDir(), nil, survey)
	if err == nil || !strings.Contains(err.Error(), "scope_drift") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateStructuredScaffoldScopeAllowsCreateMode(t *testing.T) {
	survey := WorksiteSurvey{UserOperation: userOperationCreateNewProject, ProjectState: projectStateEmptyDirectory}
	if err := validateStructuredCommandForRunWithSurvey("npm create vite@latest calculator-app -- --template react", nil, t.TempDir(), nil, survey); err != nil {
		t.Fatalf("create mode scaffold should pass scaffold policy: %v", err)
	}
}

func containsStructuredObjectiveID(objectives []StructuredObjective, id string) bool {
	for _, objective := range objectives {
		if objective.ID == id {
			return true
		}
	}
	return false
}

func TestStructuredDependencyScopeAllowsExplicitUsualStackPackages(t *testing.T) {
	workspace := t.TempDir()
	ledger := []StructuredObjective{
		{ID: "react_project", Description: "Create a React project using usual frontend stack", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true, Packages: []string{"react", "react-dom", "vite", "tailwindcss", "recyclrjs", "@hotwired/stimulus"}},
	}
	if err := validateStructuredCommandForRun("npm install react react-dom vite tailwindcss recyclrjs @hotwired/stimulus", nil, workspace, ledger); err != nil {
		t.Fatalf("explicit usual stack packages should be allowed: %v", err)
	}
}

func TestStructuredDependencyScopeAllowsRecipeRequiredPackages(t *testing.T) {
	workspace := t.TempDir()
	recipe := Recipe{
		ID: "frontend.recipe",
		Objectives: []RecipeObjective{{
			ID:          "tailwind",
			Description: "Install Tailwind",
			Packages:    []string{"tailwindcss"},
		}},
	}
	ledger := RecipeObjectiveLedger(recipe)
	if err := validateStructuredCommandForRun("npm install tailwindcss", nil, workspace, ledger); err != nil {
		t.Fatalf("recipe package should be allowed: %v", err)
	}
}

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
	}}}
	history := []Message{
		{Role: "user", Content: "What is the weather in Pattaya today?"},
		{Role: "assistant", Content: "Pattaya weather evidence."},
	}

	_, err := runStructuredCommandDecisionWithConfig(
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
		t.Fatal(err)
	}
	firstRequest := joinOllamaMessageContent(client.requests[0].Messages)
	if !strings.Contains(firstRequest, "reference_history") || !strings.Contains(firstRequest, "Pattaya") {
		t.Fatalf("follow-up did not include interpreter-approved reference history: %s", firstRequest)
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
		"worksite_survey_completed",
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
	if !structuredEventsContain(events, "structured_done_accepted") {
		t.Fatalf("missing done accepted event: %#v", events)
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

func TestStructuredCommandDecisionEvaluatorRejectsOffTrackResponseBeforeExecution(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'I do not have access to real-time information. Check the current time with a time zone app.\n'","done":false,"answer":""}`,
		`{"command":"TZ=America/New_York date '+%Y-%m-%d %H:%M:%S %Z'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Virginia is on Eastern Time."}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Confidence: 15, Feedback: "The response only prints a false limitation; use a timezone evidence command."},
		{Confidence: 95, Feedback: ""},
		{Confidence: 90, Feedback: ""},
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"what time is it in Virginia right now?",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			Evaluator:          evaluator,
			EvaluatorThreshold: 70,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("llm calls = %d, want rejected response then command then done", client.calls)
	}
	if len(evaluator.inputs) != 3 {
		t.Fatalf("evaluator calls = %d, want every llm response evaluated", len(evaluator.inputs))
	}
	if strings.Contains(stdout.String(), "I do not have access") {
		t.Fatalf("off-track response command should not execute: stdout=%q", stdout.String())
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want evaluator rejection + command", result.Observations)
	}
	first := result.Observations[0]
	if first.EvaluationConfidence != 15 || !strings.Contains(first.Stderr, "self-evaluation rejected response") {
		t.Fatalf("first observation should record evaluator rejection: %#v", first)
	}
	if first.CapabilityMemory != structuredRealtimeCapabilityMemory {
		t.Fatalf("capability memory = %q", first.CapabilityMemory)
	}
	if !structuredEventsContain(events, "structured_response_rejected") {
		t.Fatalf("missing evaluator rejection event: %#v", events)
	}
	if result.Command != "TZ=America/New_York date '+%Y-%m-%d %H:%M:%S %Z'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "Virginia is on Eastern Time." {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionDisablesUnavailableEvaluatorForTurn(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"evidence"}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{errors: []error{errors.New("model not found")}}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"produce evidence",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{Evaluator: evaluator},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want disabled after first failure", len(evaluator.inputs))
	}
	if result.Answer != "evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !structuredEventsContain(events, "structured_response_evaluator_failed") {
		t.Fatalf("missing evaluator failure event: %#v", events)
	}
}

func TestStructuredCommandDecisionDisablesContradictoryEvaluatorForTurn(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"evidence"}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Confidence: 50, Feedback: "The planner is on track and correctly answered the request."},
	}}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"produce evidence",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{Evaluator: evaluator},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want disabled after contradictory scoring", len(evaluator.inputs))
	}
	if result.Answer != "evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !structuredEventsContain(events, "structured_response_evaluator_failed") {
		t.Fatalf("missing evaluator failure event: %#v", events)
	}
}

func TestValidateStructuredCommandRejectsOnlyPureEcho(t *testing.T) {
	if err := validateStructuredCommandString("echo 'fake final answer'"); err == nil {
		t.Fatal("pure echo should be rejected")
	}
	for _, command := range []string{
		"echo 'hello' > README.md",
		"echo 'hello' | sed 's/h/H/'",
		"printf 'test evidence\n'",
	} {
		if err := validateStructuredCommandString(command); err != nil {
			t.Fatalf("command %q rejected: %v", command, err)
		}
	}
}

func TestValidateStructuredCommandRejectsMultilineScript(t *testing.T) {
	command := strings.Join([]string{
		"cd /tmp/project",
		"npm install @hotwired/stimulus",
		"npm install webpack webpack-cli --save-dev",
	}, "\n")
	err := validateStructuredCommandString(command)
	if err == nil || !strings.Contains(err.Error(), "multiline package-manager scripts are blocked") {
		t.Fatalf("multiline script should be rejected, got %v", err)
	}
	if err := validateStructuredCommandString("printf 'test evidence\n'"); err != nil {
		t.Fatalf("quoted newline command should be allowed: %v", err)
	}
	if err := validateStructuredCommandString("set -e\nprintf 'evidence'"); err != nil {
		t.Fatalf("non-package-manager script should be allowed: %v", err)
	}
}

func TestValidateStructuredCommandRequiresSpecificWTTRQuery(t *testing.T) {
	for _, command := range []string{
		"curl -s wttr.in",
		"curl -s wttr.in?format=%C",
		"curl -s wttr.in/Virginia",
	} {
		if err := validateStructuredCommandString(command); err == nil {
			t.Fatalf("command %q should be rejected", command)
		}
	}
	if err := validateStructuredCommandString("curl -s 'https://wttr.in/Virginia?format=%l|%C|%t|%f'"); err != nil {
		t.Fatalf("specific wttr command rejected: %v", err)
	}
}

func TestValidateStructuredCommandRejectsOpenWeatherMapWithoutObservedKey(t *testing.T) {
	command := `curl -s "http://api.openweathermap.org/data/2.5/weather?q=Pattaya&appid=YOUR_API_KEY&units=metric"`
	err := validateStructuredCommandString(command)
	if err == nil {
		t.Fatal("OpenWeatherMap placeholder command should be rejected")
	}
	if !strings.Contains(err.Error(), "OpenWeatherMap") || !strings.Contains(err.Error(), "wttr.in") {
		t.Fatalf("rejection should explain no-key weather source: %v", err)
	}
	if memory := structuredCapabilityMemoryForRejectedResponse(command, err.Error()); memory != structuredWeatherCapabilityMemory {
		t.Fatalf("weather capability memory = %q", memory)
	}
}

func TestValidateStructuredCommandRejectsPseudoToolsAndNone(t *testing.T) {
	for _, command := range []string{
		`web.search "current events saipan"`,
		"None",
	} {
		if err := validateStructuredCommandString(command); err == nil {
			t.Fatalf("command %q should be rejected", command)
		}
	}
}

func TestValidateStructuredCommandRequiresStableGoogleNewsRSSCurl(t *testing.T) {
	for _, command := range []string{
		`curl -s 'https://news.google.com/rss/search?q=current+events+saipan' | grep '<title>'`,
		`curl -fsSL 'https://news.google.com/rss/search?q=current+events+saipan' | grep '<title>'`,
		`curl -L 'https://news.google.com/rss/search?q=current+events+saipan&hl=en-US&gl=US&ceid=US:en' | grep '<title>'`,
	} {
		if err := validateStructuredCommandString(command); err == nil {
			t.Fatalf("Google News RSS command %q should be rejected", command)
		}
	}
	command := `curl -fsSL -A 'Mozilla/5.0' 'https://news.google.com/rss/search?q=current+events+saipan&hl=en-US&gl=US&ceid=US:en' | sed -n 's:.*<title>\([^<]*\)</title>.*:\1:p' | head -10`
	if err := validateStructuredCommandString(command); err != nil {
		t.Fatalf("stable Google News RSS command rejected: %v", err)
	}
}

func TestValidateStructuredCommandRejectsOSIdentificationWithoutPackageDiscovery(t *testing.T) {
	command := "uname -a && cat /etc/os-release"
	err := validateStructuredCommandString(command)
	if err == nil {
		t.Fatal("OS identification command without package-manager discovery should be rejected")
	}
	if !strings.Contains(err.Error(), "package-manager discovery") {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if err := validateStructuredCommandString("cat /etc/os-release && uname -srmo && command -v pacman apt dnf yum zypper apk || true"); err != nil {
		t.Fatalf("OS identification command with package-manager discovery rejected: %v", err)
	}
}

func TestValidateStructuredCommandRejectsInvalidDateTimezoneSyntax(t *testing.T) {
	for _, command := range []string{
		"date -t UTC -d 'TZ=America/New_York'",
		"date -d 'TZ=America/New_York'",
	} {
		if err := validateStructuredCommandString(command); err == nil {
			t.Fatalf("command %q should be rejected", command)
		}
	}
	for _, command := range []string{
		"TZ=America/New_York date '+%Y-%m-%d %H:%M:%S %Z'",
		"cd /tmp && TZ=America/New_York date '+%Z'",
	} {
		if err := validateStructuredCommandString(command); err != nil {
			t.Fatalf("command %q rejected: %v", command, err)
		}
	}
}

func TestRepeatedFailedStructuredCommandIncludesRejectedCommand(t *testing.T) {
	command := `curl -s "http://api.openweathermap.org/data/2.5/weather?q=Pattaya&appid=YOUR_API_KEY&units=metric"`
	observations := []StructuredCommandObservation{{
		Step:            1,
		RejectedCommand: command,
		ExitCode:        1,
		Stderr:          "shell specialist command rejected: OpenWeatherMap requires an API key",
	}}
	if !repeatedFailedStructuredCommand(command, observations) {
		t.Fatal("repeated guard should include rejected_command observations")
	}
	err := validateStructuredCommandForObservations(command, observations)
	if err == nil || !strings.Contains(err.Error(), "command repeats a previous failed command") {
		t.Fatalf("repeated rejected command should fail as repeat, got %v", err)
	}
}

func TestValidateStructuredCommandRejectsRepeatedSuccessfulCommand(t *testing.T) {
	observations := []StructuredCommandObservation{{
		Step:     1,
		Command:  "npm init -y",
		ExitCode: 0,
		Stdout:   "Wrote to package.json",
	}}
	err := validateStructuredCommandForObservations("npm   init   -y", observations)
	if err == nil || !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("repeated successful command should fail, got %v", err)
	}
}

func TestEvidenceRequiredPrerequisiteCanJustifyExecutionScope(t *testing.T) {
	workspace := t.TempDir()
	ledger := []StructuredObjective{{
		ID:              "create_calculator_ui",
		Description:     "Create missing calculator UI required before connecting UI to logic",
		Status:          "pending",
		Source:          structuredObjectiveSourceEvidenceRequiredPrerequisite,
		ParentObjective: "connect_ui_to_logic",
		Required:        true,
		Packages:        []string{"react"},
		Evidence:        "index.html missing and no existing UI entrypoint found",
	}}
	if err := validateStructuredCommandForRun("npm install react", nil, workspace, ledger); err != nil {
		t.Fatalf("evidence-required prerequisite should justify package: %v", err)
	}
	normalized, ok := normalizeStructuredObjective(ledger[0])
	if !ok || normalized.ParentObjective != "connect_ui_to_logic" {
		t.Fatalf("parent objective not preserved: %#v", normalized)
	}
}

func TestSuccessfulSetupCommandReconcilesPendingObjectiveBeforeRepeat(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"mkdir -p src/components","done":false,"answer":""}`,
		`{"command":"mkdir -p src/components","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"structure ready"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		UserOperation: userOperationModifyExisting,
		ObjectiveLedger: []StructuredObjective{
			{ID: "setup_calculator_structure", Description: "Set up calculator component structure", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "set up calculator structure", nil, client, &strings.Builder{}, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestRepeatedFailedCommandAddsAntiLoopObservation(t *testing.T) {
	workspace := t.TempDir()
	command := "printf 'install failed\\n' >&2; exit 7"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"printf 'alternate path\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"alternate path"}`,
	}}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue", nil, client, &strings.Builder{}, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, obs := range result.Observations {
		if strings.Contains(obs.Stderr, "anti_loop") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing anti-loop observation: %#v", result.Observations)
	}
}

func TestRepeatedFailedCommandForcesShellSpecialistRecovery(t *testing.T) {
	workspace := t.TempDir()
	command := "false"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"alternate path"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "test -d .",
		Rationale: "Use a different command after the planner repeated a blocked failure.",
	}}}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
			Done:   true,
			Reason: "alternate command recovered progress",
		}}},
	})
	if err != nil {
		t.Fatalf("%v observations=%#v shell_inputs=%#v stdout=%q", err, result.Observations, shell.inputs, stdout.String())
	}
	if len(shell.inputs) < 1 {
		t.Fatalf("shell specialist calls = %d, want at least 1", len(shell.inputs))
	}
	if !strings.Contains(shell.inputs[0].ToolTask, "Forbidden command(s): false") {
		t.Fatalf("recovery tool task did not include forbidden command: %q", shell.inputs[0].ToolTask)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestRepeatedSuccessfulCommandForcesCompletedEvidenceRecovery(t *testing.T) {
	workspace := t.TempDir()
	command := "ls -la " + workspace
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"inspected package"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "test -d .",
		Rationale: "Use prior ls output and inspect a new target.",
	}}}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "connect calculator UI to logic", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{
			{Done: false, Reason: "objectives still pending", ObjectiveLedger: []StructuredObjective{
				{ID: "create_calculator_ui", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "connect_ui_to_logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			}},
			{Done: false, Reason: "use completed ls evidence first", ObjectiveLedger: []StructuredObjective{
				{ID: "create_calculator_ui", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "connect_ui_to_logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			}},
			{Done: true, Reason: "recovery inspected next target", ObjectiveLedger: []StructuredObjective{
				{ID: "create_calculator_ui", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "recovery inspected next target"},
				{ID: "connect_ui_to_logic", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "recovery inspected next target"},
			}},
		}},
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{
				{ID: "create_calculator_ui", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "connect_ui_to_logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shell.inputs) < 1 {
		t.Fatalf("shell specialist calls = %d, want at least 1", len(shell.inputs))
	}
	for _, want := range []string{"Use the previous command output", command, "Do not return done=true"} {
		if !strings.Contains(shell.inputs[0].ToolTask, want) {
			t.Fatalf("recovery task missing %q: %s", want, shell.inputs[0].ToolTask)
		}
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestBlockedFalseDoneForcesRecoveryBeforeNormalPlanning(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"cat ` + filepath.ToSlash(filepath.Join(workspace, "index.html")) + `","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"","done":true,"answer":"recovered"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "test -d .",
		Rationale: "Recover from missing index.html by discovering files.",
	}}}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "connect calculator UI to logic", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
			Done:   true,
			Reason: "missing-file recovery discovered project structure",
			ObjectiveLedger: []StructuredObjective{
				{ID: "create_calculator_ui", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "discovered project structure"},
			},
		}}},
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{{ID: "create_calculator_ui", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shell.inputs) != 1 {
		t.Fatalf("shell specialist calls = %d, want 1", len(shell.inputs))
	}
	if !strings.Contains(shell.inputs[0].ToolTask, "target path does not exist") {
		t.Fatalf("missing-file recovery task = %q", shell.inputs[0].ToolTask)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestValidateStructuredCommandProtectsActiveWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "test_project_20260520115716")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		fmt.Sprintf("rm -r %q", projectDir),
		fmt.Sprintf("rmdir %q", projectDir),
		fmt.Sprintf("mv %q %q", projectDir, filepath.Join(root, "moved")),
		fmt.Sprintf("rm %q && mkdir %q", filepath.Join(root, "scratch"), filepath.Join(root, "scratch")),
	} {
		err := validateStructuredCommandForRun(command, nil, projectDir, nil)
		if err == nil {
			t.Fatalf("command %q should be rejected", command)
		}
	}
	if err := validateStructuredCommandForRun("mkdir -p . && npm init -y", nil, projectDir, nil); err != nil {
		t.Fatalf("additive initialization should be allowed: %v", err)
	}
}

func TestStructuredCommandDecisionUpdatesLedgerAfterSuccessfulCommandAndRejectsRepeat(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm init -y","done":false,"answer":""}`,
		`{"command":"npm init -y","done":false,"answer":""}`,
		`{"command":"printf 'webpack stimulus tailwind recyclr done' > setup.txt","done":false,"answer":"","objective_ledger":[{"id":"install_stimulus_js","description":"Install or account for Stimulus JS","status":"satisfied","evidence":"command output"},{"id":"install_recyclr_js","description":"Install or account for Recyclr JS","status":"satisfied","evidence":"command output"},{"id":"install_tailwind_css","description":"Install or account for Tailwind CSS","status":"satisfied","evidence":"command output"},{"id":"setup_webpack","description":"Set up webpack","status":"satisfied","evidence":"command output"}]}`,
		`{"command":"cat setup.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Project initialized and dependencies accounted for."}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "initialize_npm_project", Description: "Initialize npm project", Status: "pending"},
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "pending"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "pending"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "pending"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "pending"},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   false,
		Reason: "npm init output proves package.json was initialized",
		ObjectiveLedger: []StructuredObjective{
			{ID: "initialize_npm_project", Description: "Initialize npm project", Status: "satisfied", Evidence: "npm init wrote package.json"},
		},
	}, {
		Done:   false,
		Reason: "all objectives satisfied by command evidence and planner ledger update",
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "satisfied", Evidence: "command output"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "satisfied", Evidence: "command output"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "satisfied", Evidence: "command output"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "satisfied", Evidence: "command output"},
		},
	}, {
		Done:   true,
		Reason: "readback command verified setup.txt contents",
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "satisfied", Evidence: "cat setup.txt"},
		},
	}, {
		Done:   true,
		Reason: "readback command verified setup.txt contents",
		ObjectiveLedger: []StructuredObjective{
			{ID: "install_stimulus_js", Description: "Install or account for Stimulus JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_recyclr_js", Description: "Install or account for Recyclr JS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "install_tailwind_css", Description: "Install or account for Tailwind CSS", Status: "satisfied", Evidence: "cat setup.txt"},
			{ID: "setup_webpack", Description: "Set up webpack", Status: "satisfied", Evidence: "cat setup.txt"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"make a test project here",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: t.TempDir(),
			PromptInterpreter:       interpreter,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "cat setup.txt" {
		t.Fatalf("command = %q, want readback command retained", result.Command)
	}
	if len(checker.inputs) < 2 {
		t.Fatalf("completion checker calls = %d, want post-command and readback checks", len(checker.inputs))
	}
	if !structuredEventsContain(events, "structured_repeat_success_reconciled") {
		t.Fatalf("expected repeated command reconciliation; events=%#v", events)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionRequiresReadbackAfterPackageMutation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"readback-test","version":"1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm pkg set scripts.start='node index.js'","done":false,"answer":""}`,
		`{"command":"npm pkg get scripts.start","done":false,"answer":""}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "add_start_script", Description: "Add a start script to package.json", Status: "pending", Source: "user_explicit", Required: true},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "npm pkg set succeeded",
		ObjectiveLedger: []StructuredObjective{
			{ID: "add_start_script", Description: "Add a start script to package.json", Status: "satisfied", Evidence: "npm pkg set exited 0"},
		},
	}, {
		Done:   true,
		Reason: "npm pkg get read back the configured start script",
		ObjectiveLedger: []StructuredObjective{
			{ID: "add_start_script", Description: "Add a start script to package.json", Status: "satisfied", Evidence: "npm pkg get scripts.start returned node index.js"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"please add a start script",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "npm pkg get scripts.start" {
		t.Fatalf("final command = %q, want readback command", result.Command)
	}
	if len(checker.inputs) != 2 {
		t.Fatalf("completion checker calls = %d, want mutation and readback checks", len(checker.inputs))
	}
	if !structuredEventsContain(events, "completion_check_validation_required") {
		t.Fatalf("missing validation-required event: %#v", events)
	}
	if !strings.Contains(stdout.String(), `"node index.js"`) {
		t.Fatalf("readback stdout missing start script: %q", stdout.String())
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionSeedsLedgerFromSelectedRecipe(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'bundle evidence' > bundle.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"bundle evidence"}`,
	}}
	recipe := Recipe{
		ID:               "frontend.stimulus-tailwind-recyclr",
		Description:      "Build frontend app",
		Objectives:       []RecipeObjective{{ID: "verify_build", Description: "Verify webpack bundle"}},
		AllowedCommands:  []string{"printf"},
		EvidenceRequired: []string{"bundle exists"},
	}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RecipeIDs: []string{recipe.ID},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "bundle evidence satisfies recipe objective",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify webpack bundle", Status: "satisfied", Evidence: "bundle evidence"},
		},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"build frontend app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: t.TempDir(),
			Recipes:                 []Recipe{recipe},
			PromptInterpreter:       interpreter,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("recipe objective still pending: %#v", result.ObjectiveLedger)
	}
	if !structuredEventsContain(events, "recipe_selected") {
		t.Fatalf("missing recipe_selected event: %#v", events)
	}
}

func TestStructuredCommandDecisionAcceptsSelectedRecipeCompletionProbes(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := Recipe{
		ID:               "probe.recipe",
		Description:      "Probe recipe",
		Objectives:       []RecipeObjective{{ID: "package_json", Description: "package.json exists"}},
		AllowedCommands:  []string{"test"},
		EvidenceRequired: []string{"package.json exists"},
		CompletionChecks: []string{"test -f package.json"},
	}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		RecipeIDs: []string{recipe.ID},
	}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary: "unused",
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done: true,
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"structured recipe probe task",
		nil,
		nil,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			Recipes:                 []Recipe{recipe},
			PromptInterpreter:       interpreter,
			ContextSummarizer:       summarizer,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "RECIPE_COMPLETION_PROBES" {
		t.Fatalf("command = %q", result.Command)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("recipe objective still pending: %#v", result.ObjectiveLedger)
	}
	if !structuredEventsContain(events, "completion_check_accepted_from_recipe_probes") {
		t.Fatalf("missing recipe probe completion event: %#v", events)
	}
	if !structuredEventsContain(events, "adaptive_roles_collapsed") {
		t.Fatalf("missing adaptive role collapse event: %#v", events)
	}
	if len(summarizer.inputs) != 0 {
		t.Fatalf("context summarizer should be skipped after deterministic probes pass, calls=%d", len(summarizer.inputs))
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker should be skipped after deterministic probes pass, calls=%d", len(checker.inputs))
	}
}

func TestStructuredPayloadCommandReusesCommandCacheForUnchangedInputs(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "marker.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(workspace, ".cache")
	first := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "test -f marker.txt", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &first); err != nil {
		t.Fatal(err)
	}
	second := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"test -f marker.txt",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&second,
	); err != nil {
		t.Fatal(err)
	}
	if len(second.Observations) != 1 || !second.Observations[0].Cached {
		t.Fatalf("expected cached observation: %#v", second.Observations)
	}
	if !structuredEventsContain(events, "command_cache_hit") {
		t.Fatalf("missing command_cache_hit event: %#v", events)
	}
}

func TestStructuredPayloadCommandTimelineIncludesCommandAndOutput(t *testing.T) {
	workspace := t.TempDir()
	events := []StructuredCommandEvent{}
	result := CommandDecisionResult{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := runStructuredPayloadCommand(
		context.Background(),
		1,
		"printf 'timeline stdout\\n'",
		workspace,
		false,
		"",
		stdout,
		stderr,
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	started := structuredEventOfTypeForTest(events, "structured_command_started")
	if started == nil || started.Details["command"] == "" || started.Details["cwd"] != workspace {
		t.Fatalf("started event missing command/cwd: %#v", events)
	}
	finished := structuredEventOfTypeForTest(events, "structured_command_finished")
	if finished == nil {
		t.Fatalf("missing finished event: %#v", events)
	}
	if finished.Details["command"] == "" || finished.Details["cwd"] != workspace || finished.Details["exit_code"] != "0" {
		t.Fatalf("finished event missing command metadata: %#v", finished)
	}
	if !strings.Contains(finished.Details["stdout"], "timeline stdout") {
		t.Fatalf("finished event missing stdout: %#v", finished)
	}
	if finished.Details["stderr"] != "(empty)" {
		t.Fatalf("finished event should mark empty stderr: %#v", finished)
	}
}

func TestStructuredPayloadCommandCacheTimelineIncludesCachedOutput(t *testing.T) {
	workspace := t.TempDir()
	if _, stderr, err := runShellCommand(context.Background(), workspace, "git init && printf 'cached\\n' > marker.txt"); err != nil {
		t.Fatalf("setup git repo: %v stderr=%s", err, stderr)
	}
	cacheRoot := filepath.Join(workspace, ".cache")
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "git status --short", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}

	events := []StructuredCommandEvent{}
	cached := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"git status --short",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&cached,
	); err != nil {
		t.Fatal(err)
	}
	hit := structuredEventOfTypeForTest(events, "command_cache_hit")
	if hit == nil {
		t.Fatalf("missing command_cache_hit: %#v", events)
	}
	if hit.Details["cached"] != "true" || hit.Details["command"] == "" || hit.Details["cwd"] != workspace {
		t.Fatalf("cache hit event missing metadata: %#v", hit)
	}
	if !strings.Contains(hit.Details["stdout"], "marker.txt") {
		t.Fatalf("cache hit missing stdout: %#v", hit)
	}
	if hit.Details["stderr"] != "(empty)" {
		t.Fatalf("cache hit should mark empty stderr: %#v", hit)
	}
}

func TestStructuredPayloadCommandCacheInvalidatesWhenInputsChange(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "marker.txt")
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(workspace, ".cache")
	first := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, "test -f marker.txt", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &first); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"test -f marker.txt",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&second,
	); err != nil {
		t.Fatal(err)
	}
	if len(second.Observations) != 1 || second.Observations[0].Cached {
		t.Fatalf("expected fresh observation after input change: %#v", second.Observations)
	}
	if structuredEventsContain(events, "command_cache_hit") {
		t.Fatalf("unexpected command_cache_hit event after input change: %#v", events)
	}
}

func TestStructuredPayloadCommandDoesNotCacheFailures(t *testing.T) {
	workspace := t.TempDir()
	cacheRoot := filepath.Join(workspace, ".cache")
	first := CommandDecisionResult{}
	_ = runStructuredPayloadCommand(context.Background(), 1, "test -f missing.txt", workspace, true, cacheRoot, &bytes.Buffer{}, &bytes.Buffer{}, nil, &first)
	if first.ExitCode == 0 {
		t.Fatal("expected missing file command to have nonzero exit code")
	}
	if err := os.WriteFile(filepath.Join(workspace, "missing.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	if err := runStructuredPayloadCommand(
		context.Background(),
		2,
		"test -f missing.txt",
		workspace,
		true,
		cacheRoot,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&second,
	); err != nil {
		t.Fatal(err)
	}
	if len(second.Observations) != 1 || second.Observations[0].Cached {
		t.Fatalf("expected successful fresh observation, not cached failure: %#v", second.Observations)
	}
	if structuredEventsContain(events, "command_cache_hit") {
		t.Fatalf("unexpected command_cache_hit for prior failure: %#v", events)
	}
}

func TestStructuredCommandDecisionAppliesPatchToolArtifact(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "hello.txt")
	if err := os.WriteFile(target, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `diff --git a/hello.txt b/hello.txt
--- a/hello.txt
+++ b/hello.txt
@@ -1,2 +1,2 @@
 one
-two
+TWO
`
	response, err := json.Marshal(StructuredCommandPayload{
		Command: "",
		Done:    false,
		Answer:  "",
		Tool:    "patch.apply",
		Patch:   patch,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		string(response),
		`{"command":"test -f hello.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"updated hello.txt"}`,
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"update the file",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\nTWO\n" {
		t.Fatalf("patched file = %q", string(data))
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if !structuredEventsContain(events, "structured_patch_apply_finished") {
		t.Fatalf("missing patch apply event: %#v", events)
	}
}

func TestStructuredCommandDecisionRejectsVagueWTTRAndRetries(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"curl -s wttr.in","done":false,"answer":""}`,
		`{"command":"printf 'specific weather evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"specific weather evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "What is the weather in Virginia right now?", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want rejected wttr + specific command", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stderr, "wttr.in command must include an explicit location path") {
		t.Fatalf("first observation should reject vague wttr: %#v", result.Observations[0])
	}
	if result.Command != "printf 'specific weather evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
}

func TestStructuredCommandDecisionRejectsRepeatedFailedCommandAndRetries(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"sh -c 'exit 7'","done":false,"answer":""}`,
		`{"command":"sh -c 'exit 7'","done":true,"answer":"done"}`,
		`{"command":"printf 'fallback evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"fallback evidence"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "find evidence after a failed command", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 3 {
		t.Fatalf("observations = %#v, want failed command + repeated-command rejection + fallback command", result.Observations)
	}
	if result.Observations[1].Command != "" || !strings.Contains(result.Observations[1].Stderr, "command repeats a previous failed command") {
		t.Fatalf("second observation should reject repeated failed command: %#v", result.Observations[1])
	}
	if result.Command != "printf 'fallback evidence\n'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "fallback evidence" {
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

func TestStructuredCommandDecisionBlocksRepeatedPrematureDoneWithPendingObjectives(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"pwd","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"","done":true,"answer":"done"}`,
		`{"command":"printf 'should not run\n'","done":false,"answer":""}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "design_calculator_ui", Description: "Design calculator UI", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "verify_calculator_app", Description: "Verify calculator app", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "continue making this calculator app", nil, client, &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
	})
	if err == nil {
		t.Fatal("expected repeated premature done to stop the loop")
	}
	if _, ok := err.(CommandDecisionExhaustedError); !ok {
		t.Fatalf("err = %T %v, want CommandDecisionExhaustedError", err, err)
	}
	if client.calls != 4 {
		t.Fatalf("planner calls = %d, want stop before fifth response", client.calls)
	}
	if !result.PartialProgress {
		t.Fatal("expected partial progress after initial successful command")
	}
	if !structuredEventsContain(events, "structured_done_loop_blocked") {
		t.Fatalf("missing structured_done_loop_blocked event: %#v", events)
	}
	if got := latestStructuredFailureSummary(result.Observations); !strings.Contains(got, "anti_loop: planner returned done=true") {
		t.Fatalf("latest blocker = %q", got)
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

func TestStructuredCommandDecisionRejectsBareShellAndInstructionalDone(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "test-project-20260520")
	readmePath := filepath.Join(projectDir, "readme.md")
	command := fmt.Sprintf("mkdir -p %q && printf '# Test Project\\n' > %q && test -f %q && printf 'CREATED %s\\n'", projectDir, readmePath, readmePath, projectDir)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"bash","done":false,"answer":""}`,
		`{"command":"printf 'noop\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"To create a brand new test project with today's date in the name, you can follow these steps:\n1. Open your terminal.\n2. Navigate to ~/Projects.\n3. Run mkdir test_project_$(date +%Y%m%d)."} `,
		fmt.Sprintf(`{"command":%q,"done":false,"answer":""}`, command),
		`{"command":"","done":true,"answer":"Created the dated test project with readme.md."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := RunStructuredCommandDecisionWithEvents(
		context.Background(),
		"So in ~/Projects/ let's make a brand new test project with todays date as part of the name, and inside it just have a simple readme.md file",
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
	if result.Command != command {
		t.Fatalf("command = %q, want filesystem command", result.Command)
	}
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("readme was not created: %v", err)
	}
	if !structuredEventsContain(events, "structured_done_rejected") {
		t.Fatalf("instructional done should be rejected: %#v", events)
	}
	if !strings.Contains(result.Observations[0].Stderr, "shell/no-op launcher") {
		t.Fatalf("bare shell should be rejected first: %#v", result.Observations[0])
	}
}

func TestStructuredCommandDecisionRejectsRecursiveForceDeleteRetry(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "test_project_20260520115716")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(projectDir, "readme.md")
	if err := os.WriteFile(sentinel, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	destructive := fmt.Sprintf("rm -rf %q && mkdir %q && cd %q && npm init -y", projectDir, projectDir, projectDir)
	safe := fmt.Sprintf("mkdir -p %q && printf 'SAFE\\n' > %q", projectDir, filepath.Join(projectDir, "safe.txt"))
	client := &fakeCommandDecisionClient{responses: []string{
		fmt.Sprintf(`{"command":%q,"done":false,"answer":""}`, destructive),
		fmt.Sprintf(`{"command":%q,"done":false,"answer":""}`, safe),
		fmt.Sprintf(`{"command":%q,"done":false,"answer":""}`, "test -f "+shellQuote(filepath.Join(projectDir, "safe.txt"))+" && cat "+shellQuote(filepath.Join(projectDir, "safe.txt"))),
		`{"command":"","done":true,"answer":"Initialized safely without deleting the existing directory."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"Initialize the existing project directory without deleting existing files.",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: root},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != safe && !strings.Contains(result.Command, "safe.txt") {
		t.Fatalf("command = %q, want safe command or readback command", result.Command)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep me\n" {
		t.Fatalf("sentinel changed: content=%q err=%v", got, err)
	}
	if !structuredEventsContain(events, "structured_command_rejected") {
		t.Fatalf("expected destructive command rejection; events=%#v", events)
	}
	if len(result.Observations) == 0 || !strings.Contains(result.Observations[0].Stderr, "recursive force removal is blocked") {
		t.Fatalf("first observation should explain rm -rf rejection: %#v", result.Observations)
	}
}

func TestStructuredCommandDecisionRejectsDoneWithPendingObjectiveLedger(t *testing.T) {
	activeDir := t.TempDir()
	command := strings.Join([]string{
		"printf '%s\n' '{\"scripts\":{\"start\":\"vite\"},\"dependencies\":{\"recyclrjs\":\"latest\",\"tailwindcss\":\"latest\"}}' > package.json",
		"printf '%s\n' '<!doctype html><script src=\"https://cdn.tailwindcss.com\"></script><main id=\"calculator\">Calculator display operator operand</main><script type=\"module\">import \"recyclrjs\"; console.log(\"calculate\")</script>' > index.html",
		"test -f package.json",
		"test -f index.html",
		"grep -qi calculator index.html",
		"grep -qi tailwind index.html",
		"grep -qi recyclr package.json index.html",
		"printf 'CALCULATOR_APP_OK tailwind recyclr npm package.json index.html\n'",
	}, " && ")
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf '{\"name\":\"placeholder\"}\n' > package.json","done":false,"answer":"","objective_ledger":[{"id":"npm_project","description":"Create an npm package manifest","status":"satisfied","evidence":"package.json written"}]}`,
		`{"command":"","done":true,"answer":"npm project initialized"}`,
		`{"command":` + quoteJSONForTest(command) + `,"done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Calculator app created.","objective_ledger":[{"id":"calculator","description":"Implement calculator UI and logic","status":"satisfied","evidence":"index.html contains calculator UI and logic"},{"id":"tailwind_css","description":"Include Tailwind CSS","status":"satisfied","evidence":"index.html references Tailwind CDN"},{"id":"recyclrjs","description":"Account for RecyclrJS","status":"satisfied","evidence":"package.json/index.html reference recyclrjs"}]}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "npm_project", Description: "Create an npm package manifest", Status: "pending"},
			{ID: "calculator", Description: "Implement calculator UI and logic", Status: "pending"},
			{ID: "tailwind_css", Description: "Include Tailwind CSS", Status: "pending"},
			{ID: "recyclrjs", Description: "Account for RecyclrJS", Status: "pending"},
		},
	}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary:     "Build the calculator app in the active directory.",
		Facts:       []string{"active directory is the target project"},
		Constraints: []string{"do not use the repository root"},
		OpenItems:   []string{"finish calculator, Tailwind, and RecyclrJS objectives"},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"build a test calculator web app with recyclrjs and npm and tailwind css",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: activeDir, PromptInterpreter: interpreter, ContextSummarizer: summarizer},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != command {
		t.Fatalf("command = %q, want final app creation command", result.Command)
	}
	if !structuredEventsContain(events, "structured_done_rejected") {
		t.Fatalf("done with pending objectives should be rejected: %#v", events)
	}
	if !structuredEventsContain(events, "prompt_interpreter_completed") {
		t.Fatalf("prompt interpreter should seed objective ledger: %#v", events)
	}
	if !structuredEventsContain(events, "minimal_context_updated") {
		t.Fatalf("context summarizer should load minimal context: %#v", events)
	}
	if !strings.Contains(result.Observations[1].Stderr, "pending objective") {
		t.Fatalf("second observation should record pending objective rejection: %#v", result.Observations[1])
	}
	if _, err := os.Stat(filepath.Join(activeDir, "index.html")); err != nil {
		t.Fatalf("index.html was not created in active dir: %v", err)
	}
}

func TestStructuredCommandDecisionCanFinishFromFreshMinimalContext(t *testing.T) {
	client := &fakeCommandDecisionClient{}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "retrieve_weather_pattaya", Description: "Retrieve current Pattaya weather", Status: "pending"},
		},
	}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary: "Pattaya weather is fresh from memory.",
		Facts:   []string{"Partly Cloudy +29C humidity 76%, observed moments ago."},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "fresh memory satisfies weather objective",
		ObjectiveLedger: []StructuredObjective{
			{ID: "retrieve_weather_pattaya", Description: "Retrieve current Pattaya weather", Status: "satisfied", Evidence: "fresh minimal context"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"current weather request",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			PromptInterpreter: interpreter,
			ContextSummarizer: summarizer,
			CompletionChecker: checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 {
		t.Fatalf("planner should not be called when fresh context completes task, calls=%d", client.calls)
	}
	if result.Command != "MEMORY_CONTEXT" || result.ExitCode != 0 {
		t.Fatalf("result should finish from memory context: %#v", result)
	}
	if !strings.Contains(result.Answer, "Partly Cloudy") {
		t.Fatalf("answer missing memory fact: %q", result.Answer)
	}
	if !structuredEventsContain(events, "completion_check_accepted_from_context") {
		t.Fatalf("missing context completion event: %#v", events)
	}
}

func TestStructuredCommandDecisionDoneCheckSatisfiesSinglePendingObjective(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'Partly Cloudy +29C humidity 76%%\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Partly Cloudy +29C humidity 76%"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "retrieve_weather_pattaya", Description: "Retrieve current Pattaya weather", Status: "pending"},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "command output satisfies weather objective",
		ObjectiveLedger: []StructuredObjective{
			{ID: "retrieve_weather_pattaya", Description: "Retrieve current Pattaya weather", Status: "satisfied", Evidence: "Partly Cloudy +29C humidity 76%"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"current weather request",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			PromptInterpreter: interpreter,
			CompletionChecker: checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "Partly Cloudy +29C humidity 76%" {
		t.Fatalf("answer = %q", result.Answer)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
	if !structuredEventsContain(events, "completion_check_completed") {
		t.Fatalf("missing done-check event: %#v", events)
	}
}

func TestStructuredCommandDecisionRejectsPlannerDoneWithoutValidator(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'done evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"done evidence"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "complete_task", Description: "Complete the requested task", Status: "pending", Source: "user_explicit", Required: true},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"complete a task",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{PromptInterpreter: interpreter},
	)
	if err == nil {
		t.Fatalf("planner done should not complete without validator; result=%#v", result)
	}
	if !structuredEventsContain(events, "structured_done_rejected") {
		t.Fatalf("missing done rejection event: %#v", events)
	}
	if !strings.Contains(result.Observations[len(result.Observations)-1].Stderr, "pending objective") &&
		!strings.Contains(result.Observations[len(result.Observations)-1].Stderr, "anti_loop: planner returned done=true") {
		t.Fatalf("missing pending-objective done rejection observation: %#v", result.Observations)
	}
}

func TestStructuredCommandDecisionRejectsPlannerDoneWhenValidatorSaysNotDone(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'partial evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"partial evidence"}`,
		`{"command":"printf 'more evidence\n'","done":false,"answer":""}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "complete_task", Description: "Complete the requested task", Status: "pending", Source: "user_explicit", Required: true},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   false,
		Reason: "partial command evidence is not enough",
		ObjectiveLedger: []StructuredObjective{
			{ID: "complete_task", Description: "Complete the requested task", Status: "satisfied", Evidence: "planner overclaimed"},
		},
	}, {
		Done:   false,
		Reason: "planner done is not enough",
		ObjectiveLedger: []StructuredObjective{
			{ID: "complete_task", Description: "Complete the requested task", Status: "satisfied", Evidence: "planner overclaimed"},
		},
	}, {
		Done:   true,
		Reason: "more evidence completes the task",
		ObjectiveLedger: []StructuredObjective{
			{ID: "complete_task", Description: "Complete the requested task", Status: "satisfied", Evidence: "more evidence"},
		},
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"complete a task",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			PromptInterpreter: interpreter,
			CompletionChecker: checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "printf 'more evidence\n'" {
		t.Fatalf("final command = %q, want command after rejected planner done", result.Command)
	}
	if !structuredEventsContain(events, "structured_done_rejected") {
		t.Fatalf("missing validator done rejection event: %#v", events)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionDelegatesShellTaskToSpecialist(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Get current Pattaya time using local timezone evidence."}`,
		`{"command":"","done":true,"answer":"Pattaya time evidence"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'",
		Rationale: "Use the IANA timezone for Thailand.",
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"What time is it in Pattaya right now?",
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{ShellSpecialist: shell},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(shell.inputs) != 1 {
		t.Fatalf("shell specialist calls = %d, want 1", len(shell.inputs))
	}
	if shell.inputs[0].ToolTask != "Get current Pattaya time using local timezone evidence." {
		t.Fatalf("tool task = %q", shell.inputs[0].ToolTask)
	}
	if result.Command != "TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'" {
		t.Fatalf("command = %q", result.Command)
	}
	if !strings.Contains(stdout.String(), "ICT") && !strings.Contains(stdout.String(), "+07") {
		t.Fatalf("stdout = %q, want Thailand timezone evidence", stdout.String())
	}
	if !structuredEventsContain(events, "structured_tool_delegation_started") || !structuredEventsContain(events, "structured_tool_delegation_finished") {
		t.Fatalf("missing delegation events: %#v", events)
	}
}

func TestStructuredCommandDecisionRejectsShellDelegationWithoutSpecialist(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Get current Pattaya time."}`,
		`{"command":"printf 'fallback evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"fallback evidence"}`,
	}}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"What time is it in Pattaya right now?",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		nil,
		structuredCommandDecisionRunConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want delegation rejection + fallback command", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stderr, "shell specialist is not configured") {
		t.Fatalf("first observation should reject unavailable specialist: %#v", result.Observations[0])
	}
	if result.Answer != "fallback evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionFallsBackToShellSpecialistForEmptyCommand(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"fallback shell evidence"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "printf 'fallback shell evidence\n'",
		Rationale: "Recover from empty planner command by executing the active task.",
	}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"produce fallback shell evidence",
		nil,
		client,
		stdout,
		stderr,
		nil,
		nil,
		structuredCommandDecisionRunConfig{ShellSpecialist: shell},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(shell.inputs) != 1 {
		t.Fatalf("shell specialist calls = %d, want 1", len(shell.inputs))
	}
	if shell.inputs[0].ToolTask != "produce fallback shell evidence" {
		t.Fatalf("tool task = %q", shell.inputs[0].ToolTask)
	}
	if result.Answer != "fallback shell evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionShellSpecialistPivotsFromOpenWeatherMap(t *testing.T) {
	openWeather := `curl -s "http://api.openweathermap.org/data/2.5/weather?q=Pattaya&appid=YOUR_API_KEY&units=metric"`
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Get current Pattaya weather using no-key public evidence."}`,
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Get current Pattaya weather using no-key public evidence."}`,
		`{"command":"","done":true,"answer":"Pattaya weather evidence"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: openWeather, Rationale: "Use OpenWeatherMap current weather endpoint."},
		{Command: openWeather, Rationale: "Retry the same endpoint."},
		{Command: "printf 'Pattaya weather evidence\n'", Rationale: "Use a local deterministic stand-in for accepted evidence in the unit test."},
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"Okay, what is the weather in Pattaya right now?",
		nil,
		client,
		stdout,
		stderr,
		nil,
		nil,
		structuredCommandDecisionRunConfig{ShellSpecialist: shell},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(shell.inputs) < 3 {
		t.Fatalf("shell specialist calls = %d, want at least 3", len(shell.inputs))
	}
	if len(shell.inputs[1].Observations) == 0 || shell.inputs[1].Observations[0].RejectedCommand == "" {
		t.Fatalf("second shell call should receive rejected command feedback: %#v", shell.inputs[1].Observations)
	}
	if len(result.Observations) < 3 || !hasSuccessfulCommandObservation(result.Observations) {
		t.Fatalf("observations = %#v, want rejected commands and accepted recovery command", result.Observations)
	}
	if !strings.Contains(result.Observations[0].Stderr, "OpenWeatherMap requires an API key") {
		t.Fatalf("first rejection should call out keyed weather source: %#v", result.Observations[0])
	}
	if result.Observations[0].CapabilityMemory != structuredWeatherCapabilityMemory {
		t.Fatalf("weather memory missing from first rejection: %#v", result.Observations[0])
	}
	if !structuredObservationsContainStderr(result.Observations, "repeated command exhausted") {
		t.Fatalf("observations should block repeated delegated command: %#v", result.Observations)
	}
	if result.Answer != "Pattaya weather evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestShellCommandSpecialistRequestForWeatherForbidsOpenWeatherMap(t *testing.T) {
	req := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		Step:       1,
		UserPrompt: "Okay, what is the weather in Pattaya right now?",
		ToolTask:   "Get current Pattaya weather.",
	})
	content := joinOllamaMessageContent(req.Messages)
	for _, want := range []string{
		"wttr.in",
		"OpenWeatherMap",
		"api.openweathermap.org",
		"YOUR_API_KEY",
		"rejected_command",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("shell specialist request missing %q:\n%s", want, content)
		}
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
		"To delegate exact shell command selection, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"tool\":\"shell\",\"tool_task\":\"scoped instruction from planner authority\"}.",
		"To ask the user for needed help, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"ask\":true,\"question\":\"brief specific question\"}.",
		"If must_return_command is true, done=true is invalid; return a non-empty command or delegate with tool=shell.",
		"If must_return_command is true, ask=true is invalid; inspect or try a command first.",
		"If the latest real command succeeded, ask=true is invalid; continue, verify, or finish from evidence.",
		"Do not return done=true until at least one command has exit_code 0.",
		"If the latest command failed, return a different command instead of done=true.",
		"Use shell commands to satisfy requests; do not answer from memory when command evidence is required.",
		"Capability memory entries are durable self-correction facts about Omni capabilities; use them to avoid repeating rejected false limitations.",
		"Planner authority may delegate tool details to specialized tools; when shell syntax or system inspection is the narrow task, prefer tool=shell with a specific tool_task.",
		"Specialist team profiles define authority boundaries, allowed tools, memory permissions, and context contributions.",
		"Specialists may create evidence-backed memories; memory updates or deprioritization must be routed through memory, correction, manager, or summary specialists according to profile policy.",
		"Do not use echo to print an answer or apology.",
		"Do not use shell commands to simulate a final answer; commands must inspect files, run tools, query the web, create requested output, or verify evidence.",
		"Do not emit pseudo-tool names such as web.search, browser.search, None, or null as commands; commands execute in a real shell.",
		"Use tool_inventory to choose available terminal tools, skills, public sources, and agent roles.",
		"Never return an empty command when done=false unless delegating with tool=shell and a non-empty tool_task.",
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
		"For OS identification requests, package-manager evidence means discovery output from command -v, which, or type -p for pacman apt dnf yum zypper apk; distro-specific files such as /etc/apt/sources.list are not enough.",
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
		"For current events or news, use a concrete shell command such as curl -fsSL -A 'Mozilla/5.0' 'https://news.google.com/rss/search?q=<query>&hl=en-US&gl=US&ceid=US:en' or curl -L 'https://duckduckgo.com/html/?q=<query>'; do not emit web.search.",
		"For Google News RSS, use curl -fsSL -A 'Mozilla/5.0' 'https://news.google.com/rss/search?q=<query>&hl=en-US&gl=US&ceid=US:en'; keep the requested location in q= and parse a small number of titles.",
		"When using wttr.in, include an explicit location path and a concise format query.",
		"For current weather, prefer wttr.in with an explicit location path and concise format query, for example curl -s 'https://wttr.in/Pattaya?format=%l|%C|%t|%f'.",
		"Do not use OpenWeatherMap or api.openweathermap.org unless a real non-placeholder API key is already available in observed evidence.",
		"Never use YOUR_API_KEY, API_KEY_HERE, or invented credentials.",
		"Prefer simple curl commands that print readable evidence over fragile HTML parsing.",
		"For current time, prefer shell time/date commands or public no-key time sources.",
		"For location-specific time, produce local-time evidence for that location; do not answer from UTC unless UTC was requested.",
		"Do not use weather services as time sources.",
		"If using shell date for a location, choose an IANA timezone and prefix the command with TZ=Area/City before date.",
		"For Pattaya or any Thailand current-time request, use the IANA timezone Asia/Bangkok, for example TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'.",
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
	for _, want := range []string{`"tool_inventory"`, `"terminal_tools"`, `"public_sources"`, `"llm_roles"`, `"specialist_team"`, `"shell_rules"`, `"shell_execution_specialist"`, `"memory_specialist"`} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing tool inventory field %q: %s", want, message)
		}
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

func TestCompletedActionsFromStateDeduplicatesSuccessfulProgress(t *testing.T) {
	actions := completedActionsFromState([]StructuredObjective{
		{ID: "setup_calculator_structure", Description: "Set up calculator structure", Status: "satisfied", Evidence: "src/components exists"},
		{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending"},
	}, []StructuredCommandObservation{
		{Step: 1, Command: "mkdir -p src/components", ExitCode: 0, Stdout: "created"},
		{Step: 2, Command: "mkdir    -p   src/components", ExitCode: 0, Stdout: "created again"},
		{Step: 3, RejectedCommand: "npm install tailwindcss -D", ExitCode: 1, Stderr: "repeat failed"},
		{Step: 4, Command: "SKIPPED_REPEAT_SUCCESS: mkdir -p src/components", RejectedCommand: "mkdir -p src/components", ExitCode: 0},
	})
	if len(actions) != 2 {
		t.Fatalf("completed actions = %#v", actions)
	}
	if actions[0].Command != "mkdir -p src/components" {
		t.Fatalf("first action should be the original successful command: %#v", actions[0])
	}
	if actions[1].ObjectiveID != "setup_calculator_structure" {
		t.Fatalf("second action should be satisfied objective: %#v", actions[1])
	}
}

func TestStructuredCommandUserMessageIncludesCompletedActions(t *testing.T) {
	message := buildStructuredCommandUserMessage(
		"continue the calculator app",
		[]StructuredCommandObservation{{Step: 1, Command: "mkdir -p src/components", ExitCode: 0, Stdout: "created"}},
		t.TempDir(),
		[]StructuredObjective{
			{ID: "setup_calculator_structure", Description: "Set up calculator structure", Status: "satisfied", Evidence: "src/components exists"},
			{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending"},
		},
	)
	for _, want := range []string{
		`"completed_actions"`,
		`"loop_state"`,
		`"mkdir -p src/components"`,
		`"setup_calculator_structure"`,
		`"pending_objective_ids":["implement_calculator_logic"]`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing completed-action content %q: %s", want, message)
		}
	}
}

func TestStructuredLoopStateFlagsPrematureDoneLoop(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "design_calculator_ui", Status: "pending", Required: true, Source: structuredObjectiveSourceUserExplicit},
		{ID: "implement_calculator_logic", Status: "pending", Required: true, Source: structuredObjectiveSourceUserExplicit},
	}
	observations := []StructuredCommandObservation{
		{Step: 1, Command: "pwd", ExitCode: 0},
		{Step: 2, ExitCode: 1, Stderr: "done rejected: pending objective(s) remain: design_calculator_ui,implement_calculator_logic; run command(s) that satisfy the objective ledger before finishing"},
		{Step: 3, ExitCode: 1, Stderr: "done rejected: pending objective(s) remain: design_calculator_ui,implement_calculator_logic; run command(s) that satisfy the objective ledger before finishing"},
		{Step: 4, ExitCode: 1, Stderr: "anti_loop: planner returned done=true 3 times while the same pending objective(s) remain: design_calculator_ui,implement_calculator_logic. Stop returning done; choose a command or patch that satisfies the next pending objective."},
	}
	state := structuredLoopStateFromState(ledger, observations)
	if state.Status != "blocked" || state.RepeatKind != "premature_done" || state.RepeatCount != 3 {
		t.Fatalf("loop state = %#v", state)
	}
	if !strings.Contains(state.Instruction, "Stop returning done=true") {
		t.Fatalf("loop state instruction = %q", state.Instruction)
	}
}

func TestStructuredLoopStateCarriesForbiddenRepeatedCommand(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	observations := []StructuredCommandObservation{
		{Step: 1, Command: command, ExitCode: 1, Stderr: "npm failed"},
		{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
	}
	state := structuredLoopStateFromState([]StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}}, observations)
	if state.Status != "blocked" || state.RepeatKind != "rejected_command" || state.RepeatedCommand == "" {
		t.Fatalf("loop state = %#v", state)
	}
	if len(state.ForbiddenCommands) != 1 || state.ForbiddenCommands[0] != command {
		t.Fatalf("forbidden commands = %#v", state.ForbiddenCommands)
	}
	message := buildStructuredCommandUserMessage(
		"Please finish wiring up the UI and logic behind the calculator app",
		observations,
		t.TempDir(),
		[]StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}},
	)
	for _, want := range []string{
		`"forbidden_commands"`,
		command,
		`"recovery_instruction"`,
		`"repeated_command"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q: %s", want, message)
		}
	}
}

func TestStructuredObjectiveLedgerMergesPlannerDeclaredCriteria(t *testing.T) {
	ledger := mergeStructuredObjectiveLedger(nil, []StructuredObjective{
		{ID: "npm_project", Description: "Create an npm package manifest", Status: "satisfied", Evidence: "package.json written"},
		{ID: "calculator", Description: "Implement calculator UI and logic", Status: "pending"},
		{ID: "tailwind_css", Description: "Include Tailwind CSS", Status: "pending"},
		{ID: "recyclrjs", Description: "Account for RecyclrJS", Status: "pending"},
	})
	if got := structuredObjectiveIDs(pendingStructuredObjectives(ledger)); !sameStringSet(got, []string{"calculator", "tailwind_css", "recyclrjs"}) {
		t.Fatalf("pending objectives after partial planner update = %#v\nledger=%#v", got, ledger)
	}

	ledger = mergeStructuredObjectiveLedger(ledger, []StructuredObjective{
		{ID: "calculator", Status: "satisfied", Evidence: "index.html contains calculator UI and logic"},
		{ID: "tailwind_css", Status: "satisfied", Evidence: "index.html references Tailwind CDN"},
		{ID: "recyclrjs", Status: "satisfied", Evidence: "package.json references recyclrjs"},
	})
	if pending := pendingStructuredObjectives(ledger); len(pending) != 0 {
		t.Fatalf("ledger should be complete, pending=%#v ledger=%#v", pending, ledger)
	}
}

func TestPromptInterpreterParsesObjectiveLedger(t *testing.T) {
	interpretation, err := ParsePromptInterpretation(`{"requires_reference_history":true,"selected_recipe_ids":["frontend.stimulus-tailwind-recyclr"],"objective_ledger":[{"id":"calculator","description":"Implement calculator UI and logic","status":"pending"},{"id":"tailwind_css","description":"Include Tailwind CSS","status":"satisfied","evidence":"index.html links Tailwind"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := structuredObjectiveIDs(pendingStructuredObjectives(interpretation.ObjectiveLedger)); !sameStringSet(got, []string{"calculator"}) {
		t.Fatalf("pending objectives = %#v interpretation=%#v", got, interpretation)
	}
	if len(interpretation.RecipeIDs) != 1 || interpretation.RecipeIDs[0] != "frontend.stimulus-tailwind-recyclr" {
		t.Fatalf("recipe ids = %#v", interpretation.RecipeIDs)
	}
	if !interpretation.RequiresReferenceHistory {
		t.Fatal("requires_reference_history was not parsed")
	}
}

func TestPromptInterpreterRequestHasNoCommandsAndReturnsLedgerSchema(t *testing.T) {
	req := buildPromptInterpreterRequest(PromptInterpretationInput{
		UserPrompt:              "build a calculator app",
		CurrentWorkingDirectory: t.TempDir(),
		Recipes: []Recipe{{
			ID:               "frontend.stimulus-tailwind-recyclr",
			Description:      "Build frontend app",
			Objectives:       []RecipeObjective{{ID: "initialize_npm", Description: "Initialize npm"}},
			AllowedCommands:  []string{"npm init"},
			EvidenceRequired: []string{"package.json exists"},
		}},
	})
	content := joinOllamaMessageContent(req.Messages)
	for _, want := range []string{"prompt interpreter specialist", "structured objectives", "Do not choose shell commands", "objective_ledger", "requires_reference_history", "available_recipes", "selected_recipe_ids", "frontend.stimulus-tailwind-recyclr"} {
		if !strings.Contains(content, want) {
			t.Fatalf("interpreter request missing %q: %s", want, content)
		}
	}
	formatBlob, err := json.Marshal(req.Format)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(formatBlob), "objective_ledger") || !strings.Contains(string(formatBlob), "requires_reference_history") || !strings.Contains(string(formatBlob), "selected_recipe_ids") || strings.Contains(string(formatBlob), "command") {
		t.Fatalf("interpreter format should only require objective ledger: %s", string(formatBlob))
	}
}

func TestContextSummarizerProducesMinimalContextInventory(t *testing.T) {
	context, err := ParseMinimalContext(`{"summary":"Use the active project only.","facts":["active project is /tmp/app","active project is /tmp/app"],"constraints":["do not use repo root"],"open_items":["create calculator files"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if context.Summary != "Use the active project only." {
		t.Fatalf("summary = %q", context.Summary)
	}
	if len(context.Facts) != 1 || context.Facts[0] != "active project is /tmp/app" {
		t.Fatalf("facts not deduped: %#v", context.Facts)
	}
}

func TestContextSummarizerRequestCarriesCandidateContextButReturnsInventorySchema(t *testing.T) {
	req := buildContextSummarizerRequest(MinimalContextInput{
		UserPrompt:              "build here",
		CurrentWorkingDirectory: t.TempDir(),
		ObjectiveLedger: []StructuredObjective{
			{ID: "calculator", Description: "Build calculator", Status: "pending"},
		},
		CompletedActions: []CompletedAction{{ID: "command_mkdir_src_components", Kind: "file", Summary: "Completed command: mkdir -p src/components", Command: "mkdir -p src/components"}},
		History:          []Message{{Role: "user", Content: "prior irrelevant detail"}},
		SessionMemories: []SessionMemory{{
			Kind:    "preference",
			Content: "Prefer active directory over repo root.",
		}},
	})
	content := joinOllamaMessageContent(req.Messages)
	for _, want := range []string{"summary specialist", "minimal context inventory", "objective_ledger", "completed_actions", "reference_history", "session_memories"} {
		if !strings.Contains(content, want) {
			t.Fatalf("summarizer request missing %q: %s", want, content)
		}
	}
	formatBlob, err := json.Marshal(req.Format)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"summary", "facts", "constraints", "open_items"} {
		if !strings.Contains(string(formatBlob), want) {
			t.Fatalf("minimal context schema missing %q: %s", want, string(formatBlob))
		}
	}
}

func TestCompletionCheckerRequestAndParser(t *testing.T) {
	req := buildCompletionCheckerRequest(CompletionCheckInput{
		UserPrompt: "weather request",
		ObjectiveLedger: []StructuredObjective{
			{ID: "retrieve_weather_pattaya", Description: "Retrieve current Pattaya weather", Status: "pending"},
		},
		CompletedActions: []CompletedAction{{ID: "command_curl_weather", Kind: "command", Summary: "Completed command: curl wttr.in/Pattaya", Command: "curl wttr.in/Pattaya"}},
		MinimalContext:   MinimalContext{Summary: "Fresh weather exists."},
		CandidateAnswer:  "Partly Cloudy +29C",
	})
	content := joinOllamaMessageContent(req.Messages)
	for _, want := range []string{"done-check specialist", "objective_ledger", "completed_actions", "loop_state", "minimal_context", "candidate_answer"} {
		if !strings.Contains(content, want) {
			t.Fatalf("completion checker request missing %q: %s", want, content)
		}
	}
	check, err := ParseCompletionCheck(`{"done":true,"reason":"fresh memory","objective_ledger":[{"id":"retrieve_weather_pattaya","description":"Retrieve weather","status":"satisfied","evidence":"fresh memory"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !check.Done || len(pendingStructuredObjectives(check.ObjectiveLedger)) != 0 {
		t.Fatalf("unexpected completion check: %#v", check)
	}
}

func TestShellSpecialistRequestIncludesCompletedActions(t *testing.T) {
	req := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt: "continue the calculator app",
		ToolTask:   "choose the next command",
		CompletedActions: []CompletedAction{{
			ID:      "command_mkdir_src_components",
			Kind:    "file",
			Summary: "Completed command: mkdir -p src/components",
			Command: "mkdir -p src/components",
		}},
	})
	content := joinOllamaMessageContent(req.Messages)
	for _, want := range []string{"shell execution specialist", "completed_actions", "loop_state", "mkdir -p src/components", "never choose a command that repeats"} {
		if !strings.Contains(content, want) {
			t.Fatalf("shell specialist request missing %q: %s", want, content)
		}
	}
}

func TestEvaluatorRequestIncludesCompletedActions(t *testing.T) {
	req := buildStructuredLLMEvaluationRequest(StructuredLLMEvaluationInput{
		Step:        2,
		UserPrompt:  "continue the calculator app",
		PlannerJob:  structuredCommandPlannerJobSummary(),
		LLMResponse: `{"command":"mkdir -p src/components","done":false,"answer":""}`,
		CompletedActions: []CompletedAction{{
			ID:      "command_mkdir_src_components",
			Kind:    "file",
			Summary: "Completed command: mkdir -p src/components",
			Command: "mkdir -p src/components",
		}},
	})
	content := joinOllamaMessageContent(req.Messages)
	for _, want := range []string{"completed_actions", "loop_state", "mkdir -p src/components", "reject planner output that repeats completed work"} {
		if !strings.Contains(content, want) {
			t.Fatalf("evaluator request missing %q: %s", want, content)
		}
	}
}

func TestStructuredCommandUsesMinimalContextInsteadOfRawHistoryWhenAvailable(t *testing.T) {
	req := buildStructuredCommandRequestWithContext(
		"build here",
		[]Message{{Role: "user", Content: "raw history detail that should not be sent"}},
		nil,
		nil,
		t.TempDir(),
		nil,
		MinimalContext{Summary: "Only use active project.", Facts: []string{"active project is selected"}},
	)
	joined := joinOllamaMessageContent(req.Messages)
	if !strings.Contains(joined, "minimal_context") || !strings.Contains(joined, "Only use active project.") {
		t.Fatalf("request missing minimal context: %s", joined)
	}
	if strings.Contains(joined, "raw history detail that should not be sent") || strings.Contains(joined, "reference_history") {
		t.Fatalf("raw history leaked despite minimal context: %s", joined)
	}
}

func TestStructuredCommandUserMessageIncludesObjectiveLedger(t *testing.T) {
	message := buildStructuredCommandUserMessage(
		"build a test calculator web app with recyclrjs and npm and tailwind css",
		nil,
		t.TempDir(),
		[]StructuredObjective{
			{ID: "calculator", Description: "Implement calculator UI and logic", Status: "pending"},
			{ID: "tailwind_css", Description: "Include Tailwind CSS", Status: "pending"},
			{ID: "recyclrjs", Description: "Account for RecyclrJS", Status: "pending"},
		},
	)
	for _, want := range []string{
		`"objective_ledger"`,
		`"pending_objective_ids"`,
		`"calculator"`,
		`"tailwind_css"`,
		`"recyclrjs"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing objective ledger content %q: %s", want, message)
		}
	}
}

func TestStructuredCommandUserMessageIncludesRecipeConstraints(t *testing.T) {
	message := buildStructuredCommandUserMessage(
		"build frontend",
		nil,
		t.TempDir(),
		nil,
		MinimalContext{},
		[]Recipe{{
			ID:               "frontend.stimulus-tailwind-recyclr",
			Description:      "Build frontend app",
			AllowedCommands:  []string{"npm install", "npx webpack"},
			EvidenceRequired: []string{"dist/bundle.js exists"},
			CompletionChecks: []string{"test -f dist/bundle.js"},
		}},
	)
	for _, want := range []string{
		`"recipes"`,
		`"frontend.stimulus-tailwind-recyclr"`,
		`"allowed_commands"`,
		`"dist/bundle.js exists"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing recipe content %q: %s", want, message)
		}
	}
}

func TestStructuredCommandRequestUsesSessionActiveDirectory(t *testing.T) {
	activeDir := filepath.Join(t.TempDir(), "active-project")
	req := buildStructuredCommandRequestWithMemoriesAndCWD(
		"build the app here",
		nil,
		nil,
		nil,
		activeDir,
	)
	if len(req.Messages) != 1 {
		t.Fatalf("messages = %#v, want active task only", req.Messages)
	}
	active := req.Messages[0].Content
	escapedActiveDir := strings.Trim(quoteJSONForTest(activeDir), `"`)
	if !strings.Contains(active, `"current_working_directory":"`+escapedActiveDir+`"`) {
		t.Fatalf("active task missing session active directory %q: %s", activeDir, active)
	}
}

func TestStructuredCommandExecutesRelativeCommandsInConfiguredDirectory(t *testing.T) {
	activeDir := t.TempDir()
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"pwd; touch app.marker","done":false,"answer":""}`,
		`{"command":"pwd; test -f app.marker && ls app.marker","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"created marker"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"create a marker in the active directory",
		nil,
		client,
		stdout,
		stderr,
		nil,
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: activeDir},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command == "" || !strings.Contains(stdout.String(), activeDir) {
		t.Fatalf("command did not run in active dir: command=%q stdout=%q stderr=%q", result.Command, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(activeDir, "app.marker")); err != nil {
		t.Fatalf("marker was not created in active dir: %v", err)
	}
}

func TestStructuredCommandRequestIncludesCapabilityMemorySeparately(t *testing.T) {
	req := buildStructuredCommandRequestWithMemories(
		"What time is it in Virginia right now?",
		nil,
		[]SessionMemory{{
			Kind:      "capability",
			Content:   structuredRealtimeCapabilityMemory,
			Tags:      []string{"realtime-evidence", "capability"},
			CreatedAt: "2026-05-19T10:55:00Z",
		}},
		nil,
	)
	if len(req.Messages) != 3 {
		t.Fatalf("messages = %#v, want capability memory ack and active task", req.Messages)
	}
	if !strings.Contains(req.Messages[0].Content, `"capability_memory"`) || !strings.Contains(req.Messages[0].Content, "location-specific time") {
		t.Fatalf("capability memory message missing content: %#v", req.Messages)
	}
	activeTask := activeTaskJSONForTest(t, req.Messages[2].Content)
	if strings.Contains(activeTask, "location-specific time") {
		t.Fatalf("active task should not be polluted by capability memory: %s", activeTask)
	}
}

func TestStructuredCommandRequestIncludesCompactPrepContext(t *testing.T) {
	req := buildStructuredCommandRequestWithMemories(
		"fix Vite React routing",
		nil,
		[]SessionMemory{
			{
				Kind:    "documentation_brief",
				Content: "Documentation authority brief\nlocations:\n- Place React components in src/",
				Tags:    []string{"documentation", "vite"},
			},
			{
				Kind:    "codebase_route_brief",
				Content: "CODEBASE_ROUTE_BRIEF\nlikely_files: src/App.jsx\nverification_commands: npm test",
				Tags:    []string{"codebase-route"},
			},
		},
		nil,
	)
	joined := structuredRequestMessagesText(req)
	for _, want := range []string{"prep_context", "documentation_brief", "codebase_route_brief", "Do not let prep context add unrequested dependencies"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("request missing prep context %q:\n%s", want, joined)
		}
	}
}

func TestStructuredCommandRequestIncludesValidatedPrepBundle(t *testing.T) {
	workspace := t.TempDir()
	survey := WorksiteSurvey{WorkspacePath: workspace, ProjectState: projectStateExistingReactApp, PackageManager: packageManagerNPM}
	route := TaskRoute{Intent: "fix Vite React routing", LikelyFiles: []string{"src/App.jsx"}, VerificationCommands: []string{"npm test"}, Confidence: 80}
	bundle := NewPrepContextBundle("task", workspace, survey, ContextToolPlan{NeedsShell: true, Tools: []string{"shell"}}, route, []SessionMemory{
		{Kind: "documentation_brief", Content: "Vite components usually live under src/.", Tags: []string{"documentation", "vite"}},
	})
	req := buildStructuredCommandRequestWithContextRecipesSurveyAndPrep(
		"fix Vite React routing",
		nil,
		nil,
		nil,
		workspace,
		nil,
		MinimalContext{},
		nil,
		survey,
		bundle,
	)
	joined := structuredRequestMessagesText(req)
	for _, want := range []string{"prep_context_bundle", "prep-evidence-worksite-survey", "used_by", "shell_specialist", "Do not treat memory, documentation, or web research as execution permission"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("request missing prep bundle %q:\n%s", want, joined)
		}
	}
}

func structuredRequestMessagesText(req OllamaChatRequest) string {
	parts := make([]string, 0, len(req.Messages)+1)
	parts = append(parts, req.ContextSystem)
	for _, message := range req.Messages {
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n")
}

func structuredObservationsContainStderr(observations []StructuredCommandObservation, needle string) bool {
	for _, obs := range observations {
		if strings.Contains(obs.Stderr, needle) {
			return true
		}
	}
	return false
}

func TestParseStructuredLLMEvaluationRequiresIntegerConfidence(t *testing.T) {
	evaluation, err := ParseStructuredLLMEvaluation(`{"confidence":82,"feedback":"on track"}`)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Confidence != 82 || evaluation.Feedback != "on track" {
		t.Fatalf("evaluation = %#v", evaluation)
	}
	if evaluation.Verdict != "accept" {
		t.Fatalf("verdict = %q", evaluation.Verdict)
	}
	if _, err := ParseStructuredLLMEvaluation(`{"feedback":"missing score"}`); err == nil {
		t.Fatal("expected missing confidence error")
	}
	if _, err := ParseStructuredLLMEvaluation(`{"confidence":101,"feedback":"too high"}`); err == nil {
		t.Fatal("expected out-of-range confidence error")
	}
}

func TestParseStructuredLLMEvaluationSupportsHardVerdict(t *testing.T) {
	evaluation, err := ParseStructuredLLMEvaluation(`{"verdict":"reject","confidence":100,"blocking_reason":"scope drift","feedback":"command creates a new project"}`)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Verdict != "reject" || evaluation.Confidence != 100 || evaluation.BlockingReason != "scope drift" {
		t.Fatalf("evaluation = %#v", evaluation)
	}
}

func TestStructuredCommandDecisionFirstRequestSchemaAllowsCommandOrShellDelegation(t *testing.T) {
	format := buildStructuredCommandResponseFormat(nil)
	props := format["properties"].(map[string]interface{})
	command := props["command"].(map[string]interface{})
	done := props["done"].(map[string]interface{})
	if _, ok := command["minLength"]; ok {
		t.Fatalf("first command schema should allow empty command for tool delegation: %#v", command)
	}
	if _, ok := props["tool"]; !ok {
		t.Fatalf("first schema missing tool field: %#v", props)
	}
	if _, ok := props["tool_task"]; !ok {
		t.Fatalf("first schema missing tool_task field: %#v", props)
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

func TestPromptInterpretationDoctrineDocumentsHardBan(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "docs", "omni", "DEV_BIBLE.md"),
		filepath.Join("..", "..", "docs", "omni", "CONTRACTS.md"),
	} {
		blob, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(blob)
		for _, want := range []string{
			"No production prompt phrase matching",
			"prompt_interpreter",
			"objective_ledger",
			"minimal_context",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing doctrine marker %q", path, want)
			}
		}
	}
}

func TestObjectiveLedgerAndMinimalContextDoNotUsePromptPhraseHeuristics(t *testing.T) {
	sourcePath := filepath.Join("llm_command.go")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	forbidden := []string{
		"structuredObjectiveSpecsForPrompt",
		"buildStructuredObjectiveLedger(prompt",
		"strings.Contains(lower, \"web app\")",
		"strings.Contains(lower, \"calculator\")",
		"strings.Contains(lower, \"tailwind\")",
		"strings.Contains(lower, \"recyclr\")",
	}
	for _, needle := range forbidden {
		if strings.Contains(source, needle) {
			t.Fatalf("objective/minimal-context path contains banned prompt phrase heuristic %q", needle)
		}
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskRejectsReadOnly(t *testing.T) {
	err := validateShellProposalAgainstToolTask("ls -la src", "Required next behavior: create or modify the actual project files now. Do not continue with read-only inventory commands.")
	if err == nil {
		t.Fatal("expected read-only shell proposal to be rejected for write-required task")
	}
	if !strings.Contains(err.Error(), "requires file creation") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskAllowsMutation(t *testing.T) {
	command := "cat > index.html <<'HTML'\n<div id=\"app\"></div>\nHTML"
	if err := validateShellProposalAgainstToolTask(command, "create or modify the actual project files now"); err != nil {
		t.Fatalf("mutation command rejected: %v", err)
	}
}

func TestReconcileObjectiveLedgerSatisfiesRemovalObjective(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "remove_calculator_js", Description: "Remove src/calculator.js if it is empty and unused.", Status: "pending"},
		{ID: "run_npm_test", Description: "Run npm test after cleanup.", Status: "pending"},
	}
	events := []StructuredCommandEvent{}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "rm src/calculator.js && npm test",
		ExitCode: 0,
		Stdout:   "calculator smoke test passed",
	}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	})

	for _, id := range []string{"remove_calculator_js", "run_npm_test"} {
		found := false
		for _, objective := range updated {
			if objective.ID == id {
				found = true
				if !structuredObjectiveSatisfied(objective) {
					t.Fatalf("%s not satisfied: %#v", id, objective)
				}
			}
		}
		if !found {
			t.Fatalf("missing objective %s in %#v", id, updated)
		}
	}
	if !structuredEventsContain(events, "objective_ledger_reconciled") {
		t.Fatalf("missing reconciliation event: %#v", events)
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

func structuredEventOfTypeForTest(events []StructuredCommandEvent, eventType string) *StructuredCommandEvent {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotSet := map[string]int{}
	for _, value := range got {
		gotSet[value]++
	}
	for _, value := range want {
		if gotSet[value] == 0 {
			return false
		}
		gotSet[value]--
	}
	return true
}

func activeTaskJSONForTest(t *testing.T, raw string) string {
	t.Helper()
	var payload struct {
		ActiveTask json.RawMessage `json:"active_task"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode active payload: %v\n%s", err, raw)
	}
	if len(payload.ActiveTask) == 0 {
		t.Fatalf("missing active_task: %s", raw)
	}
	return string(payload.ActiveTask)
}
