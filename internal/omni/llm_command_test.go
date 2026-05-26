package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

type fakeCodeContentSpecialist struct {
	proposals []CodeContentProposal
	errors    []error
	inputs    []CodeContentSpecialistInput
}

type fakeCursorArchitectAgent struct {
	results []CursorArchitectAgentResult
	errors  []error
	inputs  []CursorArchitectAgentInput
	run     func(CursorArchitectAgentInput) error
}

type fakeStreamingArchitectAgent struct {
	events []AgentEvent
	inputs []CursorArchitectAgentInput
}

type fakeExternalAgentSession struct {
	events       []AgentEvent
	startedJobs  []ExternalAgentJob
	cancelCount  int
	cleanupCount int
}

type fakeUserAssistanceSpecialist struct {
	questions []UserAssistanceQuestion
	errors    []error
	inputs    []UserAssistanceInput
}

func (f *fakeUserAssistanceSpecialist) BuildUserAssistanceQuestion(ctx context.Context, input UserAssistanceInput) (UserAssistanceQuestion, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return UserAssistanceQuestion{}, err
		}
	}
	if len(f.questions) == 0 {
		return UserAssistanceQuestion{}, nil
	}
	question := f.questions[0]
	f.questions = f.questions[1:]
	return question, nil
}

func (f *fakeCodeContentSpecialist) GenerateCodeContent(ctx context.Context, input CodeContentSpecialistInput) (CodeContentProposal, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return CodeContentProposal{}, err
		}
	}
	if len(f.proposals) == 0 {
		return CodeContentProposal{Content: "export default function App() { return null; }\n", Rationale: "default fake content"}, nil
	}
	proposal := f.proposals[0]
	f.proposals = f.proposals[1:]
	return proposal, nil
}

func (f *fakeCursorArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	f.inputs = append(f.inputs, input)
	if f.run != nil {
		if err := f.run(input); err != nil {
			return CursorArchitectAgentResult{}, err
		}
	}
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return CursorArchitectAgentResult{}, err
		}
	}
	if len(f.results) == 0 {
		return CursorArchitectAgentResult{Summary: "cursor completed"}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, nil
}

func (f *fakeStreamingArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	return CursorArchitectAgentResult{Summary: "fallback should not run"}, nil
}

func (f *fakeStreamingArchitectAgent) NewExternalAgentSession(input CursorArchitectAgentInput) (ExternalAgentSession, error) {
	f.inputs = append(f.inputs, input)
	return &fakeExternalAgentSession{events: f.events}, nil
}

func (s *fakeExternalAgentSession) Start(ctx context.Context, job ExternalAgentJob) (<-chan AgentEvent, error) {
	s.startedJobs = append(s.startedJobs, job)
	ch := make(chan AgentEvent, len(s.events))
	for _, event := range s.events {
		if event.SessionID == "" {
			event.SessionID = job.SessionID
		}
		if event.Agent == "" {
			event.Agent = job.Agent
		}
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (s *fakeExternalAgentSession) Interrupt(ctx context.Context, correction HumanCorrection) error {
	return nil
}
func (s *fakeExternalAgentSession) Cancel(ctx context.Context, reason string) error {
	s.cancelCount++
	return nil
}
func (s *fakeExternalAgentSession) Pause(ctx context.Context) error  { return nil }
func (s *fakeExternalAgentSession) Resume(ctx context.Context) error { return nil }
func (s *fakeExternalAgentSession) Cleanup(ctx context.Context) error {
	s.cleanupCount++
	return nil
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
	for _, want := range []string{
		`"active_prompt_open":"What time is it in Virginia right now?"`,
		`"current_prompt":"What time is it in Virginia right now?"`,
		`"prompt":"What time is it in Virginia right now?"`,
		`"active_prompt_close":"What time is it in Virginia right now?"`,
	} {
		if !strings.Contains(active, want) {
			t.Fatalf("active prompt missing anchor %q: %s", want, active)
		}
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
		t.Fatalf("%v observations=%#v", err, result.Observations)
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
	for _, want := range []string{
		`"active_prompt_open":"What time is it in Virginia right now?"`,
		`"current_prompt":"What time is it in Virginia right now?"`,
		`"prompt":"What time is it in Virginia right now?"`,
		`"active_prompt_close":"What time is it in Virginia right now?"`,
	} {
		if !strings.Contains(active, want) {
			t.Fatalf("active prompt missing anchor %q: %s", want, active)
		}
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

func TestStructuredCommandDecisionAsksBeforeShellSpecialistDependencyInstallScopeDrift(t *testing.T) {
	workspace := t.TempDir()
	writeLocalNPMPackageForApprovalTest(t, workspace)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"install dependencies for the React project"}`,
		`{"command":"test -d node_modules/local-test-pkg","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"React project started"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "npm install file:./local-pkg --ignore-scripts --package-lock=false",
		Rationale: "install the dependency needed by the current React project",
	}}}
	userAssistance := &fakeUserAssistanceSpecialist{}
	asked := []string{}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "local package install and verification evidence passed",
	}}}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"create a new React project",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		func(ctx context.Context, question string) (string, error) {
			asked = append(asked, question)
			return "yes", nil
		},
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory:  workspace,
			ShellSpecialist:          shell,
			UserAssistanceSpecialist: userAssistance,
			CompletionChecker:        checker,
		},
	)
	if err != nil {
		t.Fatalf("approved shell dependency install should execute: %v observations=%#v", err, result.Observations)
	}
	if len(asked) != 1 {
		t.Fatalf("approval asks = %d, want 1", len(asked))
	}
	if len(userAssistance.inputs) != 1 || userAssistance.inputs[0].Kind != "dependency_install_approval" {
		t.Fatalf("user assistance inputs = %#v", userAssistance.inputs)
	}
	if !approvalTestStringSliceContains(userAssistance.inputs[0].Packages, "file:./local-pkg") {
		t.Fatalf("approval packages = %#v", userAssistance.inputs[0].Packages)
	}
	if len(result.Observations) < 2 || result.Observations[0].Question == "" || result.Observations[1].Command == "" {
		t.Fatalf("expected approval observation followed by executed install: %#v", result.Observations)
	}
}

func TestStructuredCommandDecisionAsksBeforePlannerDependencyCommandScopeDrift(t *testing.T) {
	workspace := t.TempDir()
	writeLocalNPMPackageForApprovalTest(t, workspace)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm install file:./local-pkg --ignore-scripts --package-lock=false","done":false,"answer":""}`,
		`{"command":"test -d node_modules/local-test-pkg","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"React project started"}`,
	}}
	asked := []string{}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "local package install and verification evidence passed",
	}}}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"create a new React project",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		func(ctx context.Context, question string) (string, error) {
			asked = append(asked, question)
			return "approve", nil
		},
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatalf("approved planner dependency install should execute: %v observations=%#v", err, result.Observations)
	}
	if len(asked) != 1 {
		t.Fatalf("approval asks = %d, want 1", len(asked))
	}
	if len(result.Observations) < 2 || result.Observations[0].Question == "" || !strings.Contains(result.Observations[1].Command, "npm install file:./local-pkg") {
		t.Fatalf("expected approval observation followed by planner install: %#v", result.Observations)
	}
}

func TestDependencyInstallApprovalRequiredWhenNoAskHandler(t *testing.T) {
	workspace := t.TempDir()
	writeLocalNPMPackageForApprovalTest(t, workspace)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm install file:./local-pkg --ignore-scripts --package-lock=false","done":false,"answer":""}`,
	}}
	_, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"create a new React project",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	var inputErr UserInputRequiredError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want UserInputRequiredError", err)
	}
	if !strings.Contains(inputErr.Question, "file:./local-pkg") {
		t.Fatalf("question = %q", inputErr.Question)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "node_modules", "local-test-pkg")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dependency install should not run before approval, stat err=%v", statErr)
	}
}

func TestDependencyInstallApprovalIsRememberedForSameCommand(t *testing.T) {
	command := "npm install file:./local-pkg --ignore-scripts --package-lock=false"
	observations := []StructuredCommandObservation{{
		Question:     "Allow Omnidex to install these dependencies for the current task: file:./local-pkg?\nCommand: " + command,
		UserResponse: "yes",
	}}
	if !dependencyInstallPreviouslyApproved(command, observations) {
		t.Fatal("expected prior approval to be reused")
	}
}

func TestStructuredCommandDecisionEvaluatorScopeDriftBlocksExecutionAtThreshold(t *testing.T) {
	workspace := createReactFixture(t)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"cd /home/gryph/Projects/tmp && npx create-react-app calculator-app","done":false,"answer":""}`,
		`{"command":"printf 'export default function App(){ return \"calculator\"; }\n' > src/App.js","done":false,"answer":""}`,
		`{"command":"test -s src/App.js && grep -q calculator src/App.js","done":false,"answer":""}`,
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

func TestStructuredCommandDecisionBroadEvaluatorRunsOnlyForDonePayload(t *testing.T) {
	workspace := createReactFixture(t)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'scoped command evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"scoped command evidence"}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "accept", Confidence: 100, Feedback: "final alignment only"},
	}}
	var stdout strings.Builder
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "modify existing app", nil, client, &stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		Evaluator:               evaluator,
		EvaluatorThreshold:      70,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "scoped command evidence") {
		t.Fatalf("non-final command should execute under scoped validators: %q", stdout.String())
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want final done only", len(evaluator.inputs))
	}
	if result.Answer != "scoped command evidence" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionDefersBroadEvaluatorUntilDone(t *testing.T) {
	workspace := createReactFixture(t)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'first candidate\n'","done":false,"answer":""}`,
		`{"command":"printf 'second scoped command\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"second scoped command"}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "accept", Confidence: 100, Feedback: "final alignment only"},
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
	if !strings.Contains(stdout.String(), "second scoped command") {
		t.Fatalf("second planner command did not execute before final evaluation: %q", stdout.String())
	}
	if structuredEventsContain(events, "structured_response_rejected") {
		t.Fatalf("broad evaluator should not reject non-final scoped work: %#v", events)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want final done only", len(evaluator.inputs))
	}
	if result.Answer != "second scoped command" {
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

func TestValidateCargoScaffoldRejectsNestedCurrentWorkspaceProject(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "rust-chess")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	err := validateStructuredCommandForRun("cargo new rust-chess --bin", nil, workspace, nil)
	if err == nil || !strings.Contains(err.Error(), "nested project") {
		t.Fatalf("expected nested cargo new rejection, got %v", err)
	}
	if err := validateStructuredCommandForRun("cargo init --bin", nil, workspace, nil); err != nil {
		t.Fatalf("cargo init should be allowed in active workspace: %v", err)
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

func approvalTestStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func observationsContainStderr(observations []StructuredCommandObservation, want string) bool {
	for _, observation := range observations {
		if strings.Contains(observation.Stderr, want) {
			return true
		}
	}
	return false
}

func writeLocalNPMPackageForApprovalTest(t *testing.T, workspace string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"approval-test","version":"1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(workspace, "local-pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"local-test-pkg","version":"1.0.0","main":"index.js"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "index.js"), []byte("module.exports = { ok: true };\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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

func TestStructuredDependencyScopeAllowsReactClockTailwindObjectives(t *testing.T) {
	workspace := t.TempDir()
	ledger := []StructuredObjective{
		{ID: "ensure_typical_react_structure", Description: "Ensure typical React structure for the clock app", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "install_dependencies", Description: "Install dependencies for the React clock app", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "setup_tailwind_css", Description: "Set up Tailwind CSS styling", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
	}
	if err := validateStructuredCommandForRun("npm install react react-dom tailwindcss", nil, workspace, ledger); err != nil {
		t.Fatalf("react clock dependency install should be allowed: %v", err)
	}
	if err := validateStructuredCommandForRun("npm install -D postcss autoprefixer", nil, workspace, ledger); err != nil {
		t.Fatalf("tailwind support dependency install should be allowed: %v", err)
	}
}

func TestStructuredDependencyScopeAllowsRustChessRulesLibraryObjective(t *testing.T) {
	workspace := t.TempDir()
	ledger := []StructuredObjective{
		{ID: "legal_chess_rules", Description: "Use a proven Rust chess rules library for legal move enforcement", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
	}
	if err := validateStructuredCommandForRun("cargo add shakmaty", nil, workspace, ledger); err != nil {
		t.Fatalf("rust chess rules dependency should be allowed: %v", err)
	}
	if err := validateStructuredCommandForRun("cargo add chess", nil, workspace, ledger); err != nil {
		t.Fatalf("rust chess dependency should be allowed: %v", err)
	}
}

func TestDeterministicProgressionRecoveryBuildsReactClockViteCommand(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"react":"^19.0.0","react-dom":"^19.0.0"},"devDependencies":{"tailwindcss":"^4.0.0"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	decision := ProgressionDecision{
		Action:           ProgressForceRecovery,
		Reason:           "same command/output repeated without satisfying pending objectives; no-progress recovery required",
		RecoveryToolTask: "Required next behavior: create or modify the actual project files now. Tailwind CSS Vite integration is required.",
	}

	command := deterministicProgressionRecoveryCommand("Build a React clock app with Tailwind styling and a timezone dropdown", decision, workspace)
	if command == "" {
		t.Fatal("expected deterministic React clock recovery command")
	}
	for _, want := range []string{"@tailwindcss/vite", `@import "tailwindcss"`, "src/App.jsx", "Timezone", "npm run build", "npm test"} {
		if !strings.Contains(command, want) {
			t.Fatalf("deterministic command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, "npx tailwindcss init") {
		t.Fatalf("deterministic command should not use legacy Tailwind CLI init: %s", command)
	}
}

func TestDeterministicProgressionRecoveryBuildsReactJSONFormatterCommand(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"json-test","version":"1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	decision := ProgressionDecision{
		Action:           ProgressForceRecovery,
		Reason:           "repeated command failed to advance; deterministic recovery required",
		RecoveryToolTask: "Required next behavior: create or modify the actual project files now after placeholder-only touch command failed.",
	}

	command := deterministicProgressionRecoveryCommand("Build this current directory into a React JSON formatter app", decision, workspace)
	if command == "" {
		t.Fatal("expected deterministic React JSON formatter recovery command")
	}
	for _, want := range []string{"src/jsonFormatter.js", "formatJSON", "minifyJSON", "Invalid JSON", "src/App.jsx", "npm run build", "npm test", "json formatter smoke test passed"} {
		if !strings.Contains(command, want) {
			t.Fatalf("deterministic command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, "touch index.js") {
		t.Fatalf("deterministic command should not create placeholder files: %s", command)
	}
}

func TestDeterministicProgressionRecoveryRepairsReactJSONFormatterSmokeTest(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "jsonFormatter.js"), []byte(`export function formatJSON(){}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "scripts", "smoke-test.js"), []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	decision := ProgressionDecision{
		Action:           ProgressForceRecovery,
		Reason:           "repeated command failed to advance; deterministic recovery required",
		RecoveryToolTask: "Fix SyntaxError in scripts/smoke-test.js and run build/test.",
	}

	command := deterministicProgressionRecoveryCommand("Finish React JSON formatter app; smoke-test.js failed with SyntaxError from malformed newline string", decision, workspace)
	if command == "" {
		t.Fatal("expected deterministic smoke-test repair command")
	}
	for _, want := range []string{"scripts/smoke-test.js", `\\n  "b": 2`, "npm run build", "npm test", "json formatter smoke test passed"} {
		if !strings.Contains(command, want) {
			t.Fatalf("repair command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, "value.includes('\n") {
		t.Fatalf("repair command contains literal newline in JS string: %s", command)
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

func TestStructuredCommandDecisionCancelWhileWaitingForUserInputStopsRun(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'before ask\n' >&2; exit 1","done":false,"answer":""}`,
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
		`{"command":"printf 'blocked first\n' >&2; exit 1","done":false,"answer":""}`,
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
		`{"command":"printf 'blocked first\n' >&2; exit 1","done":false,"answer":""}`,
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

func TestStructuredCommandDecisionScopedValidatorRejectsOffTrackResponseBeforeExecution(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'I do not have access to real-time information. Check the current time with a time zone app.\n'","done":false,"answer":""}`,
		`{"command":"TZ=America/New_York date '+%Y-%m-%d %H:%M:%S %Z'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Virginia is on Eastern Time."}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
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
		t.Fatalf("llm calls = %d, want rejected command, evidence command, then done", client.calls)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want final done only", len(evaluator.inputs))
	}
	if strings.Contains(stdout.String(), "I do not have access") {
		t.Fatalf("off-track response command should not execute: stdout=%q", stdout.String())
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %#v, want scoped validator rejection + command", result.Observations)
	}
	first := result.Observations[0]
	if !strings.Contains(first.Stderr, "print-only false capability limitation") {
		t.Fatalf("first observation should record scoped validator rejection: %#v", first)
	}
	if first.CapabilityMemory != structuredRealtimeCapabilityMemory {
		t.Fatalf("capability memory = %q", first.CapabilityMemory)
	}
	if !structuredEventsContain(events, "structured_evaluator_deferred_for_scoped_validation") {
		t.Fatalf("missing evaluator defer event: %#v", events)
	}
	if result.Command != "TZ=America/New_York date '+%Y-%m-%d %H:%M:%S %Z'" {
		t.Fatalf("command = %q", result.Command)
	}
	if result.Answer != "Virginia is on Eastern Time." {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestStructuredCommandDecisionDefersEvaluatorAndUsesScopedPatchValidation(t *testing.T) {
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
	patchPayload, err := json.Marshal(StructuredCommandPayload{
		Command: "",
		Done:    false,
		Tool:    "patch.apply",
		Patch:   patch,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"echo 'Step 1: edit hello.txt'","done":false,"answer":""}`,
		string(patchPayload),
		`{"command":"grep -q TWO hello.txt","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"hello.txt now contains TWO."}`,
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "accept", Confidence: 95, Feedback: "done from evidence"},
	}}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"update hello.txt to say TWO",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			Evaluator:               evaluator,
			EvaluatorThreshold:      70,
		},
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
	if !structuredEventsContain(events, "structured_command_rejected") {
		t.Fatalf("missing scoped command rejection event: %#v", events)
	}
	if !structuredEventsContain(events, "structured_patch_apply_finished") {
		t.Fatalf("missing patch apply event: %#v", events)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want final done only", len(evaluator.inputs))
	}
	if result.Answer != "hello.txt now contains TWO." {
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
	if err := validateStructuredCommandString("echo 'Step 1: Plan' && echo 'Step 2: Still planning'"); err == nil {
		t.Fatal("echo-only chains should be rejected")
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

func TestValidateStructuredCommandRejectsPrintOnlyForSubstantiveObjectives(t *testing.T) {
	ledger := []StructuredObjective{{ID: "implement_notes_app", Description: "Implement notes app UI", Status: "pending", Required: true}}
	if err := validateStructuredCommandForRun("printf 'done\n'", nil, t.TempDir(), ledger); err == nil {
		t.Fatal("print-only command should not satisfy app implementation objectives")
	}
	if err := validateStructuredCommandForRun("printf 'done\n'", nil, t.TempDir(), nil); err != nil {
		t.Fatalf("generic print evidence should remain allowed: %v", err)
	}
	if err := validateStructuredCommandString("printf 'I do not have access to real-time information. Check the current time with a time zone app.\n'"); err == nil {
		t.Fatal("print-only false capability limitation should be rejected")
	}
}

func TestPlannerRepairsEchoOnlyPlanForPendingAppObjectives(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"react":"latest","react-dom":"latest"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	substantive := "cat > src/App.js <<'EOF'\nexport default function App(){ return 'notes memory crud'; }\nEOF\n\ntest -s src/App.js"
	memoryWrite := "printf '\\n// memory state managed with useState\\n' >> src/App.js && test -s src/App.js"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"echo 'Step 1: Set up Notes Context' && echo 'Step 2: Update App.js' && echo 'Step 3: Run the Application'","done":false,"answer":"","objective_ledger":[{"id":"create_note_taking_component","description":"Create note taking component UI","status":"pending"},{"id":"implement_memory_state_management","description":"Implement memory state management","status":"pending"}]}`,
		`{"command":` + quoteJSONForTest(substantive) + `,"done":false,"answer":"","objective_ledger":[{"id":"create_note_taking_component","description":"Create note taking component UI","status":"satisfied","evidence":"src/App.js written"}]}`,
		`{"command":` + quoteJSONForTest(memoryWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"implement_memory_state_management","description":"Implement memory state management","status":"satisfied","evidence":"App contains memory marker"}]}`,
		`{"command":"","done":true,"answer":"Notes app source created."}`,
	}}
	events := []StructuredCommandEvent{}
	stdout := &bytes.Buffer{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"continue setting up this existing React project as a note app",
		nil,
		client,
		stdout,
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	if err != nil {
		t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
	}
	if strings.Contains(stdout.String(), "Step 1:") {
		t.Fatalf("echo-only plan should not execute: stdout=%q", stdout.String())
	}
	if !structuredEventsContain(events, "structured_planner_repair_started") || !structuredEventsContain(events, "structured_planner_repair_accepted") {
		t.Fatalf("missing planner repair events: %#v", events)
	}
	if client.calls < 2 || !strings.Contains(client.prompts[1], "pure echo command is not command evidence") {
		t.Fatalf("repair prompt missing echo feedback: calls=%d prompts=%#v", client.calls, client.prompts)
	}
	if _, err := os.Stat(filepath.Join(workspace, "src/App.js")); err != nil {
		t.Fatalf("expected repaired source file: %v", err)
	}
}

func TestValidateStructuredCommandNormalizesMultilineScript(t *testing.T) {
	command := strings.Join([]string{
		"cd /tmp/project",
		"npm install @hotwired/stimulus",
		"npm install webpack webpack-cli --save-dev",
	}, "\n")
	if err := validateStructuredCommandString(command); err != nil {
		t.Fatalf("multiline script should normalize and validate: %v", err)
	}
	normalized := normalizeStructuredCommandLineBreaks(command)
	want := "cd /tmp/project && npm install @hotwired/stimulus && npm install webpack webpack-cli --save-dev"
	if normalized != want {
		t.Fatalf("normalized command = %q, want %q", normalized, want)
	}
	if err := validateStructuredCommandString("printf 'test evidence\n'"); err != nil {
		t.Fatalf("quoted newline command should be allowed: %v", err)
	}
	if got := normalizeStructuredCommandLineBreaks("printf 'test evidence\n'"); got != "printf 'test evidence\n'" {
		t.Fatalf("quoted newline command was changed: %q", got)
	}
	if err := validateStructuredCommandString("set -e\nprintf 'evidence'"); err != nil {
		t.Fatalf("non-package-manager script should be allowed: %v", err)
	}
	if got := normalizeStructuredCommandLineBreaks("set -e\nprintf 'evidence'"); got != "set -e && printf 'evidence'" {
		t.Fatalf("set -e command normalized to %q", got)
	}
}

func TestNormalizeStructuredCommandAddsMkdirParents(t *testing.T) {
	command := "mkdir src/components src/pages src/hooks && touch src/App.js src/components/NoteList.js"
	want := "mkdir -p src/components src/pages src/hooks && touch src/App.js src/components/NoteList.js"
	if got := normalizeStructuredCommand(command); got != want {
		t.Fatalf("normalized command = %q, want %q", got, want)
	}
	if err := validateStructuredCommandString(command); err != nil {
		t.Fatalf("bare nested mkdir should normalize before validation: %v", err)
	}
	alreadySafe := "mkdir -p src/components && touch src/App.js"
	if got := normalizeStructuredCommand(alreadySafe); got != alreadySafe {
		t.Fatalf("mkdir -p command changed to %q", got)
	}
	withOption := "mkdir -m 755 src && touch src/App.js"
	if got := normalizeStructuredCommand(withOption); got != withOption {
		t.Fatalf("mkdir with explicit option changed to %q", got)
	}
}

func TestValidateStructuredCommandAllowsInitialPlaceholderButRejectsRepeatedPlaceholder(t *testing.T) {
	command := "mkdir src/components src/pages src/hooks && touch src/App.js src/components/NoteList.js src/hooks/useNotes.js"
	ledger := []StructuredObjective{
		{ID: "setup_note_app", Description: "Set up the note-taking app", Status: "pending"},
		{ID: "implement_crud_operations", Description: "Implement CRUD operations", Status: "pending"},
	}
	err := validateStructuredCommandForRun(command, nil, t.TempDir(), ledger)
	if err != nil {
		t.Fatalf("initial placeholder scaffold should be allowed as setup progress: %v", err)
	}
	err = validateStructuredCommandForRun("touch src/Another.js", []StructuredCommandObservation{{Step: 1, Command: command, ExitCode: 0}}, t.TempDir(), ledger)
	if err == nil {
		t.Fatal("expected repeated placeholder-only app mutation to be rejected")
	}
	if !strings.Contains(err.Error(), "placeholder-only scaffold already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	substantive := "mkdir src/components && cat > src/App.js <<'JS'\nexport default function App(){ return 'Notes'; }\nJS"
	if err := validateStructuredCommandForRun(substantive, nil, t.TempDir(), ledger); err != nil {
		t.Fatalf("substantive app write should be allowed: %v", err)
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

func TestRejectedCommandDoesNotBecomeRepeatBan(t *testing.T) {
	command := `printf 'failed\n' >&2; exit 7`
	observations := []StructuredCommandObservation{{
		Step:            1,
		RejectedCommand: command,
		ExitCode:        1,
		Stderr:          "shell specialist command rejected",
	}}
	if repeatedFailedStructuredCommand(command, observations) {
		t.Fatal("rejected command observation should not create a repeat ban")
	}
	err := validateStructuredCommandForObservations(command, observations)
	if err != nil {
		t.Fatalf("repeated rejected command should be diagnostic, not validation-blocked, got %v", err)
	}
	state := structuredLoopStateFromState([]StructuredObjective{{ID: "create_project", Status: "pending"}}, observations)
	if len(state.ForbiddenCommands) != 0 {
		t.Fatalf("forbidden commands = %#v, want none", state.ForbiddenCommands)
	}
}

func TestValidateStructuredCommandAllowsRepeatedSuccessfulCommand(t *testing.T) {
	observations := []StructuredCommandObservation{{
		Step:     1,
		Command:  "npm init -y",
		ExitCode: 0,
		Stdout:   "Wrote to package.json",
	}}
	err := validateStructuredCommandForObservations("npm   init   -y", observations)
	if err != nil {
		t.Fatalf("repeated successful command should be allowed for permissive retry policy, got %v", err)
	}
}

func TestValidateStructuredCommandForRunFlagsRepeatedSuccessfulInstall(t *testing.T) {
	command := "npm run build"
	observations := []StructuredCommandObservation{{
		Step:     15,
		Command:  command,
		ExitCode: 0,
		Stdout:   "built in 1s",
	}}
	err := validateStructuredCommandForRun(command, observations, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("repeated successful command should not be rejected by string-only validation: %v", err)
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
	command := `mkdir -p src/components && printf 'export default function Calculator(){ return null; }\n' > src/components/Calculator.jsx`
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"test -s src/components/Calculator.jsx && grep -q Calculator src/components/Calculator.jsx && printf 'source verification passed\n'","done":false,"answer":""}`,
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

func TestSourceWriteCommandReconcilesOnlySatisfiedNotesObjectives(t *testing.T) {
	command := `echo "import React, { createContext, useState } from 'react';

const NotesContext = createContext();

export const NotesProvider = ({ children }) => {
  const [notes, setNotes] = useState([]);

  const addNote = (note) => {
    setNotes([...notes, note]);
  };

  const deleteNote = (id) => {
    setNotes(notes.filter(note => note.id !== id));
  };

  return (
    <NotesContext.Provider value={{ notes, addNote, deleteNote }}>
      {children}
    </NotesContext.Provider>
  );
};" > src/hooks/useNotes.js`
	ledger := []StructuredObjective{
		{ID: "setup_notes_context", Description: "Set up Notes Context", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "update_appjs_with_notescontext", Description: "Update App.js with NotesContext", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "create_noteslist_component", Description: "Create NotesList component", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "implement_add_and_delete_note_functions", Description: "Implement add and delete note functions", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
	}

	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  command,
		ExitCode: 0,
	}, nil)

	satisfied := map[string]bool{}
	for _, objective := range updated {
		if structuredObjectiveSatisfied(objective) {
			satisfied[objective.ID] = true
		}
	}
	for _, want := range []string{"setup_notes_context", "implement_add_and_delete_note_functions"} {
		if !satisfied[want] {
			t.Fatalf("%s was not reconciled as satisfied: %#v", want, updated)
		}
	}
	for _, stillPending := range []string{"update_appjs_with_notescontext", "create_noteslist_component"} {
		if satisfied[stillPending] {
			t.Fatalf("%s should remain pending without file evidence: %#v", stillPending, updated)
		}
	}
	pendingIDs := strings.Join(structuredObjectiveIDs(pendingStructuredObjectives(updated)), ",")
	if pendingIDs != "update_appjs_with_notescontext,create_noteslist_component" {
		t.Fatalf("pending ids = %q", pendingIDs)
	}
}

func TestStructuredCommandDecisionCompletesNotesAppAfterLedgerShrinksAcrossWrites(t *testing.T) {
	workspace := t.TempDir()
	commands := []string{
		`mkdir -p src/hooks && printf '%s\n' "import React, { createContext, useState } from 'react';
const NotesContext = createContext();
export const NotesProvider = ({ children }) => {
  const [notes, setNotes] = useState([]);
  const addNote = (note) => setNotes([...notes, note]);
  const deleteNote = (id) => setNotes(notes.filter(note => note.id !== id));
  return <NotesContext.Provider value={{ notes, addNote, deleteNote }}>{children}</NotesContext.Provider>;
};
export default NotesContext;" > src/hooks/useNotes.js`,
		`mkdir -p src && printf '%s\n' "import React from 'react';
import { NotesProvider } from './hooks/useNotes';
import NotesList from './components/NotesList';
export default function App() {
  return <NotesProvider><NotesList /></NotesProvider>;
}" > src/App.js`,
		`mkdir -p src/components && printf '%s\n' "import React from 'react';
import NotesContext from '../hooks/useNotes';
export default function NotesList() {
  return <div>NotesList</div>;
}" > src/components/NotesList.js`,
		`printf '%s\n' "// addNote deleteNote implementation verified" >> src/hooks/useNotes.js`,
		`test -s src/hooks/useNotes.js && test -s src/App.js && test -s src/components/NotesList.js && grep -q addNote src/hooks/useNotes.js && grep -q deleteNote src/hooks/useNotes.js && grep -q NotesProvider src/App.js && grep -q NotesList src/components/NotesList.js && printf 'notes app verified\n'`,
	}
	responses := make([]string, 0, len(commands)+1)
	for _, command := range commands {
		payload, err := json.Marshal(StructuredCommandPayload{Command: command, Done: false})
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, string(payload))
	}
	responses = append(responses, `{"command":"","done":true,"answer":"Notes app context, App.js integration, NotesList, and add/delete functions are verified."}`)
	client := &fakeCommandDecisionClient{responses: responses}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		UserOperation: userOperationModifyExisting,
		ObjectiveLedger: []StructuredObjective{
			{ID: "setup_notes_context", Description: "Set up Notes Context", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "update_appjs_with_notescontext", Description: "Update App.js with NotesContext", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "create_noteslist_component", Description: "Create NotesList component", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "implement_add_and_delete_note_functions", Description: "Implement add and delete note functions", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"continue setting up this project as a React notes app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
		},
	)
	if err != nil {
		t.Fatalf("%v observations=%#v work_items=%#v", err, result.Observations, result.WorkItems)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
	if result.Answer == "" || !strings.Contains(result.Answer, "verified") {
		t.Fatalf("answer = %q", result.Answer)
	}
	if !structuredEventsContain(events, "partial_completion_accepted") {
		t.Fatalf("missing partial completion event: %#v", events)
	}
	if client.calls != len(responses) {
		t.Fatalf("client calls = %d, want %d", client.calls, len(responses))
	}
}

func TestRepeatedFailedCommandExecutesPermissiveRetry(t *testing.T) {
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
	if len(result.Observations) < 2 || !strings.Contains(result.Observations[1].Command, "install failed") || result.Observations[1].ExitCode != 7 {
		t.Fatalf("repeated failed command should execute as a retry under permissive policy: %#v", result.Observations)
	}
}

func TestDoneTrueWithNonEmptyDockerCommandExecutesBeforeCompletionValidation(t *testing.T) {
	workspace := t.TempDir()
	command := "test -d . && printf 'docker build ok\\ndocker run ok\\nDOCKER_SMOKE_OK running=true restarting=false restart_count=0\\nDOCKER_LOGS_CLEAR\\n'"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":true,"answer":"docker complete"}`,
		`{"command":"","done":true,"answer":"docker lifecycle verified"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "build_docker_image", Description: "Build Docker image", Status: "pending", Required: true},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{
		{Done: false, Reason: "command evidence should be gathered first"},
		{Done: true, Reason: "docker build and run evidence observed", ObjectiveLedger: []StructuredObjective{
			{ID: "build_docker_image", Description: "Build Docker image", Status: "satisfied", Required: true},
		}},
	}}
	stdout := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "Build and run the Docker image.", nil, client, stdout, &strings.Builder{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		PromptInterpreter:       interpreter,
		CompletionChecker:       checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "DOCKER_SMOKE_OK") {
		t.Fatalf("non-empty done=true command was not executed, stdout=%q observations=%#v", stdout.String(), result.Observations)
	}
	if !structuredEventsContain(events, "structured_done_ignored") {
		t.Fatalf("done=true command should emit done-ignored execution event: %#v", events)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestRepeatedFailedCommandDoesNotHardForceShellSpecialistRecovery(t *testing.T) {
	workspace := t.TempDir()
	command := "false"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"printf 'alternate path\n'","done":false,"answer":""}`,
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
	if len(shell.inputs) != 0 {
		t.Fatalf("repeated command should not hard-force shell specialist under permissive retry policy: %#v", shell.inputs)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestWriteRecoveryBypassesShellAfterRepeatedInvalidSpecialistProposals(t *testing.T) {
	decision := ProgressionDecision{
		Reason:           "workspace inspection has not produced app files; creation step is now required",
		RecoveryToolTask: "Required next behavior: create or modify the actual project files now. Do not continue with read-only inventory commands.",
	}
	result := &CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Step: 1, RejectedCommand: "touch README.md", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive file content or verification; placeholder-only command \"touch README.md\" does not satisfy it"},
		{Step: 2, RejectedCommand: "touch index.zig", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive file content or verification; placeholder-only command \"touch index.zig\" does not satisfy it"},
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command: "touch src/main.zig",
	}}}
	events := []StructuredCommandEvent{}
	handled, err := runProgressionGateRecovery(context.Background(), 3, "Build a Rust CLI calculator", decision, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: t.TempDir(),
		ShellSpecialist:         shell,
	}, WorksiteSurvey{}, &strings.Builder{}, &strings.Builder{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, nil, result)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("expected recovery to fall through to planner after repeated invalid shell proposals")
	}
	if len(shell.inputs) != 0 {
		t.Fatalf("shell specialist should be bypassed: %#v", shell.inputs)
	}
	if !structuredEventsContain(events, "progression_gate_shell_bypassed") {
		t.Fatalf("missing shell bypass event: %#v", events)
	}
}

func TestWriteRecoveryBypassesShellAfterRepeatedDocumentationDownloadProposals(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Step: 1, RejectedCommand: "curl -s https://ziglang.org/documentation/master/ > zig_doc.html", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive source/build/test files; documentation download command does not satisfy it"},
		{Step: 2, RejectedCommand: "curl -s https://ziglang.org/documentation/master/ > zig_doc.html", ExitCode: 1, Stderr: "shell specialist command rejected: tool_task requires substantive source/build/test files; documentation download command does not satisfy it"},
	}
	if !shouldBypassShellSpecialistForWriteRecovery("Required next behavior: create or modify the actual project files now with substantive source/build/test files.", observations) {
		t.Fatal("expected repeated documentation-download proposals to bypass shell specialist")
	}
}

func TestPlannerAcceptsInitialPlaceholderThenForcesSubstantiveContinuation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"react":"latest","react-dom":"latest"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	substantive := strings.Join([]string{
		"mkdir -p src/components",
		"cat > src/components/NoteManager.js <<'EOF'\nexport default function NoteManager(){ return 'notes crud memory'; }\nEOF",
		"test -s src/components/NoteManager.js",
	}, " && ")
	crudWrite := "printf '\\n// crud operations implemented\\n' >> src/components/NoteManager.js && test -s src/components/NoteManager.js"
	memoryWrite := "printf '\\n// memory storage implemented\\n' >> src/components/NoteManager.js && test -s src/components/NoteManager.js"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"mkdir -p src/components && touch src/components/NoteManager.js","done":false,"answer":"","objective_ledger":[{"id":"create_note_manager_component","description":"Create notes component structure","status":"pending"},{"id":"implement_crud_operations","description":"Implement CRUD operations","status":"pending"},{"id":"store_notes_in_memory","description":"Store notes in memory","status":"pending"}]}`,
		`{"command":` + quoteJSONForTest(substantive) + `,"done":false,"answer":"","objective_ledger":[{"id":"create_note_manager_component","description":"Create notes component structure","status":"satisfied","evidence":"NoteManager component file written"}]}`,
		`{"command":` + quoteJSONForTest(crudWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"implement_crud_operations","description":"Implement CRUD operations","status":"satisfied","evidence":"NoteManager contains CRUD marker"}]}`,
		`{"command":` + quoteJSONForTest(memoryWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"store_notes_in_memory","description":"Store notes in memory","status":"satisfied","evidence":"NoteManager contains memory marker"}]}`,
		`{"command":"","done":true,"answer":"Notes component created."}`,
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"please continue setting up this project as a react js note app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
	)
	if err != nil {
		t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
	}
	if structuredEventsContain(events, "structured_planner_repair_started") {
		t.Fatalf("initial placeholder scaffold should execute as setup progress, not enter repair: %#v", events)
	}
	if !structuredEventsContain(events, "partial_completion_accepted") {
		t.Fatalf("missing partial completion continuation after placeholder scaffold: %#v", events)
	}
	if structuredEventsContain(events, "structured_command_rejected") {
		t.Fatalf("initial placeholder scaffold should not be rejected: %#v", events)
	}
	if len(result.Observations) < 2 || !strings.Contains(result.Observations[0].Command, "touch src/components/NoteManager.js") {
		t.Fatalf("expected placeholder scaffold then substantive write observations: %#v", result.Observations)
	}
	if !strings.Contains(result.Observations[1].Command, "cat > src/components/NoteManager.js") {
		t.Fatalf("expected second observation to expand placeholder with substantive content: %#v", result.Observations)
	}
	if _, err := os.Stat(filepath.Join(workspace, "src/components/NoteManager.js")); err != nil {
		t.Fatalf("expected component file: %v", err)
	}
}

func TestShellSpecialistRepairsRejectedDependencyScopeDriftLocally(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"dependencies":{"react":"latest","react-dom":"latest"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"create the notes UI component and in-memory CRUD behavior using existing React dependencies"}`,
		`{"command":"","done":true,"answer":"Notes UI created."}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "npm install react-router-dom", Rationale: "Add routing."},
		{Command: "cat > src/NoteManager.jsx <<'EOF'\nexport default function NoteManager(){ return 'notes crud memory'; }\nEOF\n\ntest -s src/NoteManager.jsx", Rationale: "Write the requested component using existing dependencies."},
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"continue setting up this existing React project as a note app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		func(ctx context.Context, question string) (string, error) { return "no", nil },
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace, ShellSpecialist: shell},
	)
	if err != nil {
		t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
	}
	if len(shell.inputs) != 2 {
		t.Fatalf("shell specialist calls = %d, want local repair call", len(shell.inputs))
	}
	if len(shell.inputs[1].Observations) == 0 || !observationsContainStderr(shell.inputs[1].Observations, "dependency scope drift") {
		t.Fatalf("repair input missing validator feedback: %#v", shell.inputs[1].Observations)
	}
	if !structuredEventsContain(events, "structured_tool_delegation_repair_started") || !structuredEventsContain(events, "structured_tool_delegation_repair_accepted") {
		t.Fatalf("missing shell repair events: %#v", events)
	}
	if result.Command != "cat > src/NoteManager.jsx <<'EOF'\nexport default function NoteManager(){ return 'notes crud memory'; }\nEOF\n\ntest -s src/NoteManager.jsx" {
		t.Fatalf("command = %q", result.Command)
	}
	if _, err := os.Stat(filepath.Join(workspace, "src/NoteManager.jsx")); err != nil {
		t.Fatalf("expected notes component: %v", err)
	}
}

func TestShellSpecialistStopsLocalRepairAfterRepeatedRejectedCommand(t *testing.T) {
	result := &CommandDecisionResult{}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "npm install react-router-dom", Rationale: "Add routing."},
		{Command: "npm install react-router-dom", Rationale: "Retry routing."},
		{Command: "cat > src/NoteManager.jsx <<'EOF'\nexport default function NoteManager(){ return 'notes'; }\nEOF", Rationale: "Too late."},
	}}
	events := []StructuredCommandEvent{}
	_, ok, err := proposeValidatedShellCommand(
		context.Background(),
		4,
		"continue notes app",
		"create the notes UI component and in-memory CRUD behavior using existing React dependencies",
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: t.TempDir(), ShellSpecialist: shell},
		WorksiteSurvey{},
		&[]StructuredObjective{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		func(ctx context.Context, question string) (string, error) { return "no", nil },
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected repeated rejected proposal to stop local shell repair")
	}
	if len(shell.inputs) != 2 {
		t.Fatalf("shell calls = %d, want stop after repeated rejection", len(shell.inputs))
	}
	if shell.inputs[0].RepairAttempt != 0 || shell.inputs[1].RepairAttempt != 1 {
		t.Fatalf("repair attempts = %#v", shell.inputs)
	}
	if !structuredEventsContain(events, "structured_tool_delegation_repair_repeated") {
		t.Fatalf("missing repeated repair event: %#v", events)
	}
}

func TestShellSpecialistRequestRaisesTemperatureOnlyForRepairAttempt(t *testing.T) {
	initial := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{ToolTask: "inspect"})
	if got := initial.Options["temperature"]; got != 0 {
		t.Fatalf("initial temperature = %#v, want 0", got)
	}
	repair := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{ToolTask: "inspect", RepairAttempt: 1})
	if got := repair.Options["temperature"]; got != defaultShellSpecialistRepairTemperature {
		t.Fatalf("repair temperature = %#v, want %#v", got, defaultShellSpecialistRepairTemperature)
	}
	if !strings.Contains(repair.Messages[1].Content, `"repair_attempt":1`) {
		t.Fatalf("repair prompt missing repair_attempt: %s", repair.Messages[1].Content)
	}
}

func TestRepeatedSuccessfulCommandSkipsAndUsesCompletedEvidence(t *testing.T) {
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
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "inspect workspace and verify it exists", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{
			{Done: false, Reason: "objectives still pending", ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			}},
			{Done: false, Reason: "use completed ls evidence first", ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			}},
			{Done: true, Reason: "recovery inspected next target", ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "ls output"},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "test -d ."},
			}},
		}},
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_workspace", Description: "Inspect workspace", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
				{ID: "verify_workspace_exists", Description: "Verify workspace exists", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	successCount := 0
	skipCount := 0
	for _, obs := range result.Observations {
		if obs.Command == command && obs.ExitCode == 0 {
			successCount++
		}
		if strings.HasPrefix(obs.Command, "SKIPPED_REPEAT_SUCCESS:") && obs.RejectedCommand == command {
			skipCount++
		}
	}
	if successCount != 1 || skipCount != 1 {
		t.Fatalf("expected repeated successful command to execute once and skip once, success=%d skip=%d observations=%#v", successCount, skipCount, result.Observations)
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
	result, err := runStructuredCommandDecisionWithConfig(context.Background(), "inspect project structure", nil, client, stdout, &strings.Builder{}, nil, nil, structuredCommandDecisionRunConfig{
		CurrentWorkingDirectory: workspace,
		ShellSpecialist:         shell,
		CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
			Done:   true,
			Reason: "missing-file recovery discovered project structure",
			ObjectiveLedger: []StructuredObjective{
				{ID: "inspect_project_structure", Description: "Inspect project structure", Status: "satisfied", Source: structuredObjectiveSourceUserExplicit, Required: true, Evidence: "discovered project structure"},
			},
		}}},
		PromptInterpreter: &fakePromptInterpreter{interpretations: []PromptInterpretation{{
			ObjectiveLedger: []StructuredObjective{{ID: "inspect_project_structure", Description: "Inspect project structure", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true}},
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

func TestDelegatedShellExecutionFailureRepairsWithResponsibleShellSpecialist(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "App.js"), []byte("export default function App(){ return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"inspect existing React source files before editing"}`,
		`{"command":"","done":true,"answer":"source inspected"}`,
	}}
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{
		{Command: "cat src/components/ActualButMissing.js", Rationale: "Inspect suspected component."},
		{Command: "cat src/components/ActualButMissing.js", Rationale: "Retry suspected component."},
		{Command: "find src -maxdepth 3 -type f", Rationale: "Discover actual source files after missing path feedback."},
	}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"inspect this React app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			ShellSpecialist:         shell,
			CompletionChecker: &fakeCompletionChecker{checks: []CompletionCheck{{
				Done:   true,
				Reason: "bounded source discovery ran after missing-file feedback",
			}}},
		},
	)
	if err != nil {
		t.Fatalf("run failed: %v observations=%#v", err, result.Observations)
	}
	if len(shell.inputs) != 3 {
		t.Fatalf("shell calls = %d, want initial plus local repair attempts", len(shell.inputs))
	}
	if !observationsContainStderr(shell.inputs[1].Observations, "No such file or directory") {
		t.Fatalf("responsible shell specialist did not receive execution failure: %#v", shell.inputs[1].Observations)
	}
	if !observationsContainStderr(shell.inputs[2].Observations, "repeats the latest failed execution") {
		t.Fatalf("responsible shell specialist did not receive repeat rejection: %#v", shell.inputs[2].Observations)
	}
	if result.Command != "find src -maxdepth 3 -type f" {
		t.Fatalf("final command = %q", result.Command)
	}
	if !structuredEventsContain(events, "structured_command_rejected") {
		t.Fatalf("missing direct shell rejection event: %#v", events)
	}
}

func TestValidateShellProposalRejectsMissingFileRecoveryInvalidReadRepeat(t *testing.T) {
	toolTask := "Recovery required. A read/inspect command failed because the target path does not exist. Invalid command: cat src/components/ActualButMissing.js. Failure: step=18 command=cat src/components/ActualButMissing.js exit_code=1 stderr=cat: src/components/ActualButMissing.js: No such file or directory Required next behavior: inspect the parent directory, run a bounded file discovery command, inspect package.json if present, update the workspace model, then continue with discovered files. Do not retry the invalid path unless new evidence proves it exists."
	err := validateShellProposalAgainstToolTask("cat src/components/ActualButMissing.js", toolTask)
	if err == nil || !strings.Contains(err.Error(), "must not retry invalid read command") {
		t.Fatalf("expected invalid read retry rejection, got %v", err)
	}
	if err := validateShellProposalAgainstToolTask("find src -maxdepth 3 -type f", toolTask); err != nil {
		t.Fatalf("bounded discovery should pass missing-file recovery validation: %v", err)
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

func TestStructuredCommandDecisionDoesNotUseDoneCheckToCloseWeakLegacyLedger(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"npm init -y","done":false,"answer":""}`,
		`{"command":"npm init -y","done":false,"answer":""}`,
		`{"command":"printf 'webpack stimulus tailwind recyclr done' > setup.txt","done":false,"answer":"","objective_ledger":[{"id":"install_stimulus_js","description":"Install or account for Stimulus JS","status":"satisfied","evidence":"command output"},{"id":"install_recyclr_js","description":"Install or account for Recyclr JS","status":"satisfied","evidence":"command output"},{"id":"install_tailwind_css","description":"Install or account for Tailwind CSS","status":"satisfied","evidence":"command output"},{"id":"setup_webpack","description":"Set up webpack","status":"satisfied","evidence":"command output"}]}`,
		`{"command":"test -s setup.txt && cat setup.txt","done":false,"answer":""}`,
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
	if err == nil {
		t.Fatalf("weak legacy ledger updates should not satisfy typed queue evidence; result=%#v events=%#v", result, events)
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker should not close weak pending work: %#v", checker.inputs)
	}
	repeatedNpmInit := 0
	skippedNpmInit := 0
	for _, obs := range result.Observations {
		if normalizeStructuredCommandForComparison(obs.Command) == "npm init -y" && obs.ExitCode == 0 {
			repeatedNpmInit++
		}
		if strings.HasPrefix(obs.Command, "SKIPPED_REPEAT_SUCCESS:") && normalizeStructuredCommandForComparison(obs.RejectedCommand) == "npm init -y" {
			skippedNpmInit++
		}
	}
	if repeatedNpmInit != 1 || skippedNpmInit != 1 {
		t.Fatalf("expected repeated npm init to execute once and skip once, executed=%d skipped=%d observations=%#v events=%#v", repeatedNpmInit, skippedNpmInit, result.Observations, events)
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
	if len(checker.inputs) != 1 {
		t.Fatalf("completion checker calls = %d, want final check only after readback evidence", len(checker.inputs))
	}
	if !strings.Contains(stdout.String(), `"node index.js"`) {
		t.Fatalf("readback stdout missing start script: %q", stdout.String())
	}
	if len(checker.inputs[0].Observations) == 0 || !strings.Contains(checker.inputs[0].Observations[len(checker.inputs[0].Observations)-1].Command, "npm pkg get scripts.start") {
		t.Fatalf("completion checker should run after readback evidence: %#v", checker.inputs[0].Observations)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionSeedsLedgerFromSelectedRecipe(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'bundle evidence' > bundle.txt","done":false,"answer":""}`,
		`{"command":"test -s bundle.txt","done":false,"answer":""}`,
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
		Done: false,
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify webpack bundle", Status: "pending"},
		},
	}, {
		Done:   true,
		Reason: "bundle evidence satisfies recipe objective",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify webpack bundle", Status: "satisfied", Evidence: "test -s bundle.txt"},
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

func TestStructuredCommandDecisionAllowsRepeatedFailedCommandRetry(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"sh -c 'exit 7'","done":false,"answer":""}`,
		`{"command":"sh -c 'exit 7'","done":false,"answer":""}`,
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
		t.Fatalf("observations = %#v, want failed command + repeated failed command + fallback command", result.Observations)
	}
	if result.Observations[1].Command != "sh -c 'exit 7'" || result.Observations[1].ExitCode != 7 {
		t.Fatalf("second observation should execute repeated failed command under permissive retry policy: %#v", result.Observations[1])
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
	tailwindWrite := "printf '\\n<!-- tailwind verified -->\\n' >> index.html && grep -qi tailwind index.html"
	recyclrWrite := "printf '\\n<!-- recyclr verified -->\\n' >> index.html && grep -qi recyclr index.html"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf '{\"name\":\"placeholder\"}\n' > package.json","done":false,"answer":"","objective_ledger":[{"id":"npm_project","description":"Create an npm package manifest","status":"satisfied","evidence":"package.json written"}]}`,
		`{"command":"","done":true,"answer":"npm project initialized"}`,
		`{"command":` + quoteJSONForTest(command) + `,"done":false,"answer":""}`,
		`{"command":` + quoteJSONForTest(tailwindWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"tailwind_css","description":"Include Tailwind CSS","status":"satisfied","evidence":"index.html references Tailwind CDN"}]}`,
		`{"command":` + quoteJSONForTest(recyclrWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"recyclrjs","description":"Account for RecyclrJS","status":"satisfied","evidence":"package.json/index.html reference recyclrjs"}]}`,
		`{"command":"","done":true,"answer":"Calculator app created.","objective_ledger":[{"id":"calculator","description":"Implement calculator UI and logic","status":"satisfied","evidence":"index.html contains calculator UI and logic"}]}`,
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
	if result.Command != recyclrWrite {
		t.Fatalf("command = %q, want final queued write command", result.Command)
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

func TestStructuredCommandDecisionDoesNotRunDoneCheckFromFreshMinimalContextBeforeQueuePasses(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":true,"answer":"Partly Cloudy +29C humidity 76%"}`,
	}}
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
	if err == nil {
		t.Fatalf("fresh context should not bypass the typed work queue; result=%#v", result)
	}
	if client.calls == 0 {
		t.Fatalf("planner should still be called when the work queue has not passed")
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker ran before the typed queue passed: %#v", checker.inputs)
	}
	if structuredEventsContain(events, "completion_check_accepted_from_context") {
		t.Fatalf("context completion should not be accepted before queue evidence: %#v", events)
	}
}

func TestStructuredCommandDecisionCompletesSingleObjectiveFromTypedEvidenceWithoutDoneCheck(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"test -d . && printf 'Partly Cloudy +29C humidity 76%%\n'","done":false,"answer":""}`,
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
	if result.Command == "" {
		t.Fatalf("expected typed queue evidence command to run: %#v", result)
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker should not be needed to satisfy typed command evidence: %#v", checker.inputs)
	}
	if structuredEventsContain(events, "completion_check_completed") {
		t.Fatalf("done-check event should not satisfy the queue: %#v", events)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger should be satisfied from typed command evidence: %#v", pending)
	}
}

func TestCompletionCheckerDoneCannotSatisfyPendingObjectivesFromRationale(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "integrate_tailwindcss", Description: "Integrate Tailwind CSS", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "configure_tailwindcss_vite", Description: "Configure Tailwind CSS with Vite", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "add_package_scripts", Description: "Add package scripts", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
	}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "build output, vite config, Tailwind import, and package scripts prove completion",
		ObjectiveLedger: []StructuredObjective{
			{ID: "integrate_tailwind_css", Description: "Integrate Tailwind CSS", Status: "satisfied", Evidence: "src/style.css imports Tailwind"},
			{ID: "configure_tailwind_css_vite", Description: "Configure Tailwind CSS with Vite", Status: "satisfied", Evidence: "vite.config.js uses @tailwindcss/vite"},
		},
	}}}
	events := []StructuredCommandEvent{}

	updated, accepted := runCompletionCheck(
		context.Background(),
		3,
		"Build a React clock app with Tailwind",
		t.TempDir(),
		ledger,
		MinimalContext{},
		[]StructuredCommandObservation{{Step: 2, Command: "npm run build && npm test", ExitCode: 0, Stdout: "clock smoke test passed"}},
		"clock smoke test passed",
		checker,
		WorksiteSurvey{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if accepted {
		t.Fatalf("validator rationale should not be accepted as proof; updated=%#v events=%#v", updated, events)
	}
	if pending := pendingStructuredObjectives(updated); len(pending) != 3 {
		t.Fatalf("pending objectives should remain open without exact evidence: %#v", pending)
	}
	if structuredEventsContain(events, "completion_check_satisfied_pending_objectives") {
		t.Fatalf("natural-language satisfaction event should not occur: %#v", events)
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

func TestStructuredCommandDecisionDoesNotUseDoneCheckToSatisfyQueue(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"test -d . && printf 'partial evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"partial evidence"}`,
		`{"command":"test -d . && printf 'more evidence\n'","done":false,"answer":""}`,
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
	if result.Command == "" {
		t.Fatalf("expected typed queue evidence command to run: %#v", result)
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker should not run to satisfy the queue: %#v", checker.inputs)
	}
	if structuredEventsContain(events, "completion_check_completed") {
		t.Fatalf("done-check should not have been used for queue satisfaction: %#v", events)
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

func TestStructuredCommandDecisionArchitectLaneWritesTestThenImplementationBeforeEvaluator(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"test -s src/App.js && test -s src/App.test.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Implementation architect target root: react-music-production. Create or modify the actual project files for the React music production app."}`,
		`{"command":"","done":true,"answer":"React music production app implemented"}`,
	}}
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{Content: "import { defineConfig } from 'vite';\nimport react from '@vitejs/plugin-react';\nexport default defineConfig({ plugins: [react()] });\n", Rationale: "vite config"},
		{Content: `<div id="root"></div><script type="module" src="/src/main.jsx"></script>` + "\n", Rationale: "html shell"},
		{Content: "import React from 'react';\nimport { createRoot } from 'react-dom/client';\nimport App from './App.js';\ncreateRoot(document.getElementById('root')).render(<App />);\n", Rationale: "mount entry"},
		{Content: "import fs from 'node:fs';\nconst app = fs.readFileSync('src/App.js','utf8');\nif (!app.includes('Sequencer') || !app.includes('Tempo') || !app.includes('Studio')) process.exit(1);\n", Rationale: "test first"},
		{Content: "import React, { useState } from 'react';\nexport default function App() { const [tempo,setTempo]=useState(128); return React.createElement('main', { className: 'studio' }, React.createElement('button', { type: 'button' }, 'Transport'), React.createElement('input', { type: 'range', value: tempo, onChange: e=>setTempo(e.target.value) }), React.createElement('section', null, 'Music Studio Sequencer Channel Rack Mixer Tempo Tracks')); }\n", Rationale: "implementation after test"},
		{Content: ".studio { display: grid; } .channel-rack { color: white; } .mixer { color: white; } .timeline { color: white; }\n", Rationale: "style"},
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "accept", Confidence: 100, Feedback: "final alignment"},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{Done: false, Reason: "stale completion checker should not override typed architect evidence"}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"build a React music production app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodeContentSpecialist:   code,
			Evaluator:               evaluator,
			EvaluatorThreshold:      70,
			CompletionChecker:       checker,
		},
	)
	if err != nil {
		t.Fatalf("%v observations=%#v events=%#v", err, result.Observations, events)
	}
	if len(code.inputs) < 2 {
		t.Fatalf("code specialist calls = %d, want at least test then implementation", len(code.inputs))
	}
	if code.inputs[0].TestFirst || code.inputs[0].WorkItem.Path != "vite.config.js" {
		t.Fatalf("first code item should be Vite config after deterministic package metadata: %#v", code.inputs[0])
	}
	if !code.inputs[3].TestFirst || code.inputs[3].WorkItem.Path != "scripts/smoke-test.mjs" {
		t.Fatalf("fourth code item should be test-first smoke probe: %#v", code.inputs[3])
	}
	if code.inputs[4].TestFirst || code.inputs[4].WorkItem.Path != "src/App.js" {
		t.Fatalf("fifth code item should be implementation App.js: %#v", code.inputs[4])
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want only final broad evaluator after architect-scoped validators pass", len(evaluator.inputs))
	}
	if evaluator.inputs[0].ValidationScope != "alignment_after_typed_recursive_completion" {
		t.Fatalf("evaluator scope = %q, want final typed-completion alignment", evaluator.inputs[0].ValidationScope)
	}
	if len(checker.inputs) != 0 {
		t.Fatalf("completion checker calls = %d, want typed final gate to avoid stale LLM done veto", len(checker.inputs))
	}
	if !structuredEventsContain(events, "structured_evaluator_bypassed_for_architect") {
		t.Fatalf("missing architect evaluator bypass event: %#v", events)
	}
	if !structuredEventsContain(events, "completion_check_accepted_from_typed_final_gate") {
		t.Fatalf("missing typed final gate acceptance event: %#v", events)
	}
	appTest, err := os.ReadFile(filepath.Join(app, "scripts", "smoke-test.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	appJS, err := os.ReadFile(filepath.Join(app, "src", "App.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appTest), "Sequencer") || !strings.Contains(string(appJS), "Tempo") {
		t.Fatalf("unexpected files: test=%q app=%q", string(appTest), string(appJS))
	}
	if result.Answer != "React music production app implemented" {
		t.Fatalf("answer = %q", result.Answer)
	}
}

func TestArchitectWorkItemRequiresCurrentRunEvidenceNotExistingFile(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "src", "App.js"), []byte("export default function OldApp(){ return null; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	item := ArchitectWorkItem{ID: "create_react_entrypoint", Operation: "update", CWD: "react-music-production", Path: "src/App.js"}
	if architectWorkItemSatisfied(item, workspace, nil) {
		t.Fatal("pre-existing file content must not satisfy an architect update item without current-run evidence")
	}
	if !architectWorkItemSatisfied(item, workspace, []StructuredCommandObservation{{
		Command:  "architect.apply update react-music-production/src/App.js",
		ExitCode: 0,
	}}) {
		t.Fatal("current-run architect.apply evidence should satisfy the architect update item")
	}
}

func TestLatestFailedCommandOutputTargetsFailedProofCommand(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Command: "reject placeholder", ExitCode: 1, Stderr: "placeholder-only command does not satisfy app objectives"},
		{Command: "cd react-music-production && npm run build", ExitCode: 1, Stderr: "Failed to resolve /src/main.js from /tmp/index.html"},
	}
	got := latestFailedCommandOutput(observations, "cd react-music-production && npm run build")
	if !strings.Contains(got, "Failed to resolve /src/main.js") {
		t.Fatalf("latest failed command output = %q", got)
	}
	if strings.Contains(got, "placeholder-only") {
		t.Fatalf("proof feedback should not use stale validator text: %q", got)
	}
}

func TestReactUIBuildEvidenceSatisfiesStudioObjectives(t *testing.T) {
	obs := StructuredCommandObservation{
		Command:  "npm run build",
		ExitCode: 0,
		Stdout:   "dist/index.html\n✓ built in 80ms",
	}
	for _, objective := range []StructuredObjective{
		{ID: "create_entrypoint", Description: "Create the React entrypoint"},
		{ID: "setup_pattern_step_sequencer", Description: "Set up the pattern step sequencer"},
		{ID: "create_channel_rack", Description: "Create the channel rack"},
		{ID: "implement_mixer_controls", Description: "Implement mixer controls"},
		{ID: "develop_transport_controls", Description: "Develop transport controls"},
	} {
		if !structuredObservationSatisfiesObjective(obs, objective) {
			t.Fatalf("build evidence did not satisfy %#v", objective)
		}
	}
}

func TestNPMBuildCountsAsPostWriteValidation(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Command: "npm install", ExitCode: 0},
		{Command: "npm run build", ExitCode: 0, Stdout: "✓ built in 78ms"},
	}
	ledger := []StructuredObjective{{
		ID:          "react_music_app",
		Description: "Build the React music production app",
		Status:      "satisfied",
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
	}}
	if structuredCompletionNeedsPostWriteValidation("build a React music production app", ledger, observations) {
		t.Fatal("npm run build after npm install should satisfy post-write validation")
	}
	if !deterministicCompletionEnforcerAcceptsDone("build a React music production app", ledger, observations) {
		t.Fatal("deterministic completion enforcer should accept done after a passing npm build")
	}
}

func TestShouldDeferBroadEvaluatorForArchitectCompletionWhileRepairPending(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Command: "architect.apply update package.json", ExitCode: 0},
		{Command: "cd . && npm run build", ExitCode: 1, Stderr: "Failed to resolve /src/main.jsx from /tmp/index.html"},
	}
	payload := StructuredCommandPayload{Done: true, Answer: "done"}
	if !shouldDeferBroadEvaluatorForArchitectCompletion(payload, "build a React music production app", t.TempDir(), WorksiteSurvey{PackageManager: packageManagerNPM}, observations) {
		t.Fatal("expected broad evaluator to defer while architect repair item is pending")
	}
}

func TestStructuredCommandDecisionArchitectLaneRunsProofBeforeFinalEvaluator(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"test -s src/App.js && test -s src/App.test.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"","done":false,"answer":"","tool":"shell","tool_task":"Implementation architect target root: react-music-production. Create or modify the actual project files for the React music production app."}`,
		`{"command":"","done":true,"answer":"React music production app implemented and verified","objective_ledger":[{"id":"react_music_app","description":"Build the React music production app","status":"satisfied","source":"user_explicit","required":true,"evidence":"architect applied test, implementation, style, and npm run build passed"}]}`,
	}}
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{Content: `{"scripts":{"test":"node scripts/smoke-test.mjs","build":"test -s src/App.js && test -s scripts/smoke-test.mjs"},"dependencies":{},"devDependencies":{}}` + "\n", Rationale: "package metadata"},
		{Content: "import { defineConfig } from 'vite';\nimport react from '@vitejs/plugin-react';\nexport default defineConfig({ plugins: [react()] });\n", Rationale: "vite config"},
		{Content: `<div id="root"></div><script type="module" src="/src/main.jsx"></script>` + "\n", Rationale: "html shell"},
		{Content: "import React from 'react';\nimport { createRoot } from 'react-dom/client';\nimport App from './App.js';\ncreateRoot(document.getElementById('root')).render(<App />);\n", Rationale: "mount entry"},
		{Content: "import fs from 'node:fs';\nconst app = fs.readFileSync('src/App.js','utf8');\nif (!app.includes('Transport') || !app.includes('Tempo') || !app.includes('Studio')) process.exit(1);\n", Rationale: "proof first"},
		{Content: "import React, { useState } from 'react';\nexport default function App() { const [tempo,setTempo]=useState(128); return React.createElement('main', { className: 'studio' }, React.createElement('button', { type: 'button' }, 'Transport'), React.createElement('input', { type: 'range', value: tempo, onChange: e=>setTempo(e.target.value) }), React.createElement('section', null, 'Music Studio Sequencer Channel Rack Mixer Tempo Tracks')); }\n", Rationale: "implementation"},
		{Content: ".studio { display: grid; } .channel-rack { color: white; } .mixer { color: white; } .timeline { color: white; }\n", Rationale: "style"},
	}}
	evaluator := &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Verdict: "accept", Confidence: 100, Feedback: "final alignment"},
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{{
			ID:          "react_music_app",
			Description: "Build the React music production app",
			Status:      "pending",
			Source:      structuredObjectiveSourceUserExplicit,
			Required:    true,
		}},
	}}}
	events := []StructuredCommandEvent{}

	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"build a React music production app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			PromptInterpreter:       interpreter,
			CodeContentSpecialist:   code,
			Evaluator:               evaluator,
			EvaluatorThreshold:      70,
		},
	)
	if err != nil {
		t.Fatalf("%v observations=%#v work_items=%#v events=%#v", err, result.Observations, result.WorkItems, events)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want only final evaluator after recursive typed completion; result=%#v observations=%#v events=%#v", len(evaluator.inputs), result, result.Observations, events)
	}
	if !structuredEventsContain(events, "architect_work_item_verified") {
		t.Fatalf("missing architect proof verification event: %#v", events)
	}
	if !strings.Contains(result.Command, "npm run build") {
		t.Fatalf("final command = %q, want proof command retained", result.Command)
	}
	if gate := EvaluateTypedFinalGate(TypedFinalGateInput{Items: result.WorkItems, CompletionDone: true}); !gate.Passed {
		t.Fatalf("typed final gate did not pass after architect proof: %#v work_items=%#v", gate, result.WorkItems)
	}
}

func TestImplementationArchitectContractCarriesResearchAndDocumentationBriefs(t *testing.T) {
	workspace := t.TempDir()
	contract := buildImplementationArchitectContract(
		"build a React music app",
		"Implementation architect target root: . Create or modify the actual project files.",
		workspace,
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	prep := PrepContextBundle{
		MemoryBriefs:        []PrepBrief{{ID: "mem-1", Kind: "validated_playbook", Content: "Prior React app playbook", Tags: []string{"react"}}},
		DocumentationBriefs: []PrepBrief{{ID: "doc-1", Kind: "documentation_brief", Content: "React components belong under src/.", Tags: []string{"react", "documentation"}}},
		WebResearchBriefs:   []PrepBrief{{ID: "web-1", Kind: "web_research_brief", Content: "React docs were checked today.", Tags: []string{"react"}}},
		WebResearchChecked:  true,
	}

	enriched := enrichImplementationArchitectContract(contract, "build a React music app", "", prep, []SessionMemory{{
		Kind:    "documentation_research",
		Content: "Vite build scripts use npm run build.",
		Tags:    []string{"vite", "documentation"},
	}})

	if len(enriched.ResearchRequests) < 3 {
		t.Fatalf("research requests = %#v", enriched.ResearchRequests)
	}
	for _, specialist := range []string{"memory_retrieval_specialist", "documentation_specialist", "web_research_specialist"} {
		if !architectResearchRequestsContainSpecialist(enriched.ResearchRequests, specialist) {
			t.Fatalf("missing %s request: %#v", specialist, enriched.ResearchRequests)
		}
	}
	if len(enriched.DocumentationBriefs) == 0 || !strings.Contains(enriched.DocumentationBriefs[0].Content, "React components") {
		t.Fatalf("missing documentation brief: %#v", enriched.DocumentationBriefs)
	}
	if len(enriched.MemoryBriefs) < 2 {
		t.Fatalf("memory briefs should include prep and session memories: %#v", enriched.MemoryBriefs)
	}
	for _, brief := range append(append([]PrepBrief{}, enriched.DocumentationBriefs...), enriched.MemoryBriefs...) {
		if !stringListContains(brief.UsedBy, "implementation_architect") || !stringListContains(brief.UsedBy, "documentation_specialist") {
			t.Fatalf("brief missing architect/documentation collaboration UsedBy: %#v", brief)
		}
	}
}

func architectResearchRequestsContainSpecialist(requests []ArchitectResearchRequest, specialist string) bool {
	for _, request := range requests {
		if request.Specialist == specialist {
			return true
		}
	}
	return false
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
		{Command: "printf 'Pattaya weather evidence\n' | tee weather.txt", Rationale: "Use a local deterministic stand-in for accepted evidence in the unit test."},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{Done: true, Reason: "unit test accepted fallback weather evidence"}}}
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
		structuredCommandDecisionRunConfig{ShellSpecialist: shell, CompletionChecker: checker},
	)
	if err != nil {
		t.Fatalf("%v observations=%#v", err, result.Observations)
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
	if structuredObservationsContainStderr(result.Observations, "forbidden") {
		t.Fatalf("observations should not turn rejected delegated commands into forbidden commands: %#v", result.Observations)
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
		"For npm React TypeScript demos, prefer a minimal Vite project with package.json and src files; create-react-app is discouraged but not a hard ban when the active task explicitly asks to create a new React app.",
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

func TestStructuredCommandUserMessageIncludesTDDDevelopmentLoop(t *testing.T) {
	message := buildStructuredCommandUserMessage(
		"add note editing to the React app",
		nil,
		t.TempDir(),
		[]StructuredObjective{{ID: "implement_note_editing", Description: "Implement note editing", Status: "pending"}},
	)
	for _, want := range []string{
		`"development_loop"`,
		"test_first",
		"implement_second",
		"verify_third",
		"fallback_probe",
		"completion_gate",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("active task message missing TDD policy %q: %s", want, message)
		}
	}
}

func TestPromptInterpreterFallbackObjectiveForEmptyExecutionLedger(t *testing.T) {
	objective, ok := fallbackPromptInterpretationObjective(
		"fix the existing app",
		PromptInterpretation{UserOperation: userOperationFixExisting},
		WorksiteSurvey{},
	)
	if !ok {
		t.Fatal("expected fallback objective")
	}
	if objective.ID != "complete_active_task" || objective.Status != "pending" || objective.Source != structuredObjectiveSourceUserExplicit || !objective.Required {
		t.Fatalf("objective = %#v", objective)
	}
}

func TestStructuredCommandSystemContextIncludesTDDPolicy(t *testing.T) {
	context := buildStructuredCommandSystemContext()
	for _, want := range []string{
		"test-driven loop",
		"focused failing test",
		"deterministic verification probe",
		"Do not mark implementation objectives satisfied from a source write alone",
		"proof_plan contract",
		"Validated proof tests/probes are protected",
		"Validated playbook memories",
		"advisory acceleration only",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("system context missing %q", want)
		}
	}
}

func TestCompactStructuredPrepMemoriesIncludesValidatedPlaybookSummary(t *testing.T) {
	memories := []SessionMemory{{
		Kind: validatedPlaybookKind,
		Content: `{
			"name": "react_notes",
			"task_pattern": "build a React notes app",
			"command_sequence": ["write source", "npm run build"],
			"validation_signals": ["npm run build"],
			"confidence": 91,
			"scope_policy": "advisory_only"
		}`,
		Tags: []string{"validated-playbook", "react"},
	}}
	prep := compactStructuredPrepMemories(memories, 1)
	if len(prep) != 1 {
		t.Fatalf("prep memories=%d", len(prep))
	}
	if !strings.Contains(prep[0].Content, "commands=write source -> npm run build") {
		t.Fatalf("playbook was not summarized: %s", prep[0].Content)
	}
}

func TestStructuredCommandUserMessageIncludesProofPolicy(t *testing.T) {
	message := buildStructuredCommandUserMessage("build a notes app", nil, t.TempDir(), []StructuredObjective{{
		ID:       "create_notes_crud",
		Status:   "pending",
		Source:   structuredObjectiveSourceUserExplicit,
		Required: true,
	}})
	for _, want := range []string{
		`"proof_policy"`,
		"contract_first_tdd_loop",
		`"proof_plan_allowed_sources"`,
		structuredObjectiveSourceUserExplicit,
		structuredObjectiveSourceRecipeRequired,
		structuredObjectiveSourceEvidenceRequiredPrerequisite,
		`"proof_lifecycle"`,
		structuredProofEventTestValidated,
		structuredProofEventTestModificationRejected,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("active task message missing proof policy %q: %s", want, message)
		}
	}
}

func TestStructuredCommandResponseFormatIncludesProofPlanContract(t *testing.T) {
	format := buildStructuredCommandResponseFormat(nil)
	properties, ok := format["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("format properties missing: %#v", format)
	}
	if _, ok := properties["proof_plan"]; !ok {
		t.Fatalf("format missing proof_plan schema: %#v", properties)
	}
}

func TestParseStructuredCommandPayloadIncludesProofPlan(t *testing.T) {
	payload, err := ParseStructuredCommandPayload(`{
		"command": "npm test -- --run",
		"done": false,
		"answer": "",
		"proof_plan": {
			"objective_id": "create_notes_crud",
			"proof_type": "smoke_test",
			"commands": ["npm test -- --run"],
			"acceptance_checks": ["user can create a note"]
		}
	}`)
	if err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if payload.ProofPlan.ObjectiveID != "create_notes_crud" || payload.ProofPlan.ProofType != structuredProofTypeSmokeTest {
		t.Fatalf("proof plan not parsed: %#v", payload.ProofPlan)
	}
}

func TestShellSpecialistRequestIncludesTDDPolicy(t *testing.T) {
	req := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt:     "add note editing to the React app",
		ToolTask:       "write source files for app component CRUD objectives",
		RepairFeedback: "placeholder-only scaffold already exists; expand it",
	})
	text := structuredRequestMessagesText(req)
	for _, want := range []string{
		"repair_feedback",
		"placeholder-only scaffold already exists",
		"direct validator feedback",
		"For app/code feature tool_tasks, prefer a TDD command",
		"focused test",
		"deterministic source-verification probe",
		"validated test/probe",
		"memory_suggested",
		"After implementation writes",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("shell specialist request missing %q: %s", want, text)
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

func TestStructuredLoopStateCarriesRepeatedCommandAsEvidenceOnly(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	observations := []StructuredCommandObservation{
		{Step: 1, Command: command, ExitCode: 1, Stderr: "npm failed"},
		{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
	}
	state := structuredLoopStateFromState([]StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}}, observations)
	if state.Status != "stuck" || state.RepeatKind != "rejected_command" || state.RepeatedCommand == "" {
		t.Fatalf("loop state = %#v", state)
	}
	if len(state.ForbiddenCommands) != 0 {
		t.Fatalf("forbidden commands = %#v, want none", state.ForbiddenCommands)
	}
	message := buildStructuredCommandUserMessage(
		"Please finish wiring up the UI and logic behind the calculator app",
		observations,
		t.TempDir(),
		[]StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}},
	)
	for _, want := range []string{
		command,
		`"recovery_instruction"`,
		`"repeated_command"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q: %s", want, message)
		}
	}
	if strings.Contains(message, `"forbidden_commands":[`) {
		t.Fatalf("message should not carry observation-derived forbidden commands: %s", message)
	}
}

func TestRejectedReactScaffoldDoesNotCreateForbiddenCommand(t *testing.T) {
	rejected := "npx create-react-app notes-app"
	observations := []StructuredCommandObservation{{
		Step:            1,
		RejectedCommand: rejected,
		ExitCode:        1,
		Stderr:          "validator rejected before execution",
	}}
	state := structuredLoopStateFromState([]StructuredObjective{{ID: "initialize_new_react_project", Status: "pending"}}, observations)
	if len(state.ForbiddenCommands) != 0 {
		t.Fatalf("forbidden commands = %#v, want none", state.ForbiddenCommands)
	}
	for _, command := range []string{
		rejected,
		"npm create vite@latest notes-app -- --template react",
	} {
		if err := validateStructuredCommandForObservations(command, observations); err != nil {
			t.Fatalf("command %q should remain valid because rejected proposals are not completed actions: %v", command, err)
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
	interpretation, err := ParsePromptInterpretation(`{"requires_reference_history":true,"selected_recipe_ids":["frontend.stimulus-tailwind-recyclr"],"objective_ledger":[{"id":"calculator","description":"Implement calculator UI and logic","status":"pending","kind":"architect"},{"id":"tailwind_css","description":"Include Tailwind CSS","status":"satisfied","kind":"verify","evidence":"index.html links Tailwind"}]}`)
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
	if interpretation.ObjectiveLedger[0].Kind != string(WorkItemKindArchitect) {
		t.Fatalf("objective kind = %q", interpretation.ObjectiveLedger[0].Kind)
	}
}

func TestStructuredObjectiveMergePreservesKind(t *testing.T) {
	ledger := mergeStructuredObjectiveLedger(nil, []StructuredObjective{{
		ID:          "build_react_app",
		Description: "Build React app",
		Status:      "pending",
		Kind:        string(WorkItemKindArchitect),
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
	}})
	ledger = mergeStructuredObjectiveLedger(ledger, []StructuredObjective{{
		ID:       "build_react_app",
		Status:   "satisfied",
		Evidence: "architect queue passed",
	}})

	if len(ledger) != 1 {
		t.Fatalf("ledger = %#v", ledger)
	}
	if ledger[0].Kind != string(WorkItemKindArchitect) {
		t.Fatalf("kind was not preserved after merge: %#v", ledger[0])
	}
}

func TestPromptInterpreterRepairsTruncatedJSON(t *testing.T) {
	interpretation, err := ParsePromptInterpretation(`{"requires_reference_history":false,"objective_ledger":[{"id":"build_react_app","description":"Build React app","status":"pending"}`)
	if err != nil {
		t.Fatalf("expected repaired interpretation: %v", err)
	}
	if len(interpretation.ObjectiveLedger) != 1 || interpretation.ObjectiveLedger[0].ID != "build_react_app" {
		t.Fatalf("ledger = %#v", interpretation.ObjectiveLedger)
	}
}

func TestPromptInterpreterFallbackBuildsReactObjective(t *testing.T) {
	interpretation := fallbackPromptInterpretation("Build a React JS music production app", WorksiteSurvey{ProjectState: projectStateEmptyDirectory})
	if len(interpretation.ObjectiveLedger) != 1 {
		t.Fatalf("ledger = %#v", interpretation.ObjectiveLedger)
	}
	if interpretation.ObjectiveLedger[0].ID != "build_react_app" {
		t.Fatalf("objective = %#v", interpretation.ObjectiveLedger[0])
	}
	if interpretation.ObjectiveLedger[0].Kind != string(WorkItemKindArchitect) {
		t.Fatalf("objective kind = %q", interpretation.ObjectiveLedger[0].Kind)
	}
	if interpretation.UserOperation != userOperationCreateNewProject {
		t.Fatalf("operation = %q", interpretation.UserOperation)
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
	for _, want := range []string{"prompt interpreter specialist", "structured objectives", "Do not choose shell commands", "objective_ledger", "kind=read|create|update|delete|verify|architect", "requires_reference_history", "available_recipes", "selected_recipe_ids", "frontend.stimulus-tailwind-recyclr", "Return one compact JSON object only"} {
		if !strings.Contains(content, want) {
			t.Fatalf("interpreter request missing %q: %s", want, content)
		}
	}
	formatBlob, err := json.Marshal(req.Format)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(formatBlob), "objective_ledger") || !strings.Contains(string(formatBlob), `"kind"`) || !strings.Contains(string(formatBlob), "requires_reference_history") || !strings.Contains(string(formatBlob), "selected_recipe_ids") || strings.Contains(string(formatBlob), "command") {
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

func TestStructuredPlannerAndShellEncourageDocumentationForUnfamiliarToolchains(t *testing.T) {
	plannerReq := buildStructuredCommandRequest("Build a Zig CLI calculator.", nil, nil)
	plannerContent := strings.ToLower(plannerReq.ContextSystem + "\n" + joinOllamaMessageContent(plannerReq.Messages))
	for _, want := range []string{"unfamiliar language", "official docs", "smallest hello-world project", "source verification fallback", "tool=patch.apply"} {
		if !strings.Contains(plannerContent, want) {
			t.Fatalf("planner request missing %q: %s", want, plannerContent)
		}
	}

	shellReq := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt: "Build a Zig CLI calculator.",
		ToolTask:   "Required next behavior: create or modify the actual project files now for an unfamiliar language/toolchain.",
	})
	shellContent := strings.ToLower(joinOllamaMessageContent(shellReq.Messages))
	for _, want := range []string{"official documentation", "installed tool help", "substantive source/build/test files", "deterministic source verification fallback"} {
		if !strings.Contains(shellContent, want) {
			t.Fatalf("shell specialist request missing %q: %s", want, shellContent)
		}
	}
}

func TestShellSpecialistUsesExistingDocumentationBriefInsteadOfRefetching(t *testing.T) {
	req := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt: "Build a Zig CLI calculator.",
		ToolTask:   "Required next behavior: create or modify the actual project files now with substantive source/build/test files.",
		SessionMemories: []SessionMemory{{
			Kind:    "documentation_brief",
			Content: "Zig docs say zig init creates build.zig and src/main.zig.",
		}},
	})
	content := strings.ToLower(joinOllamaMessageContent(req.Messages))
	for _, want := range []string{"documentation_brief", "do not fetch the same docs again", "write substantive source/build/test files"} {
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

func TestStructuredCommandRequestBudgetsLargeObservationContext(t *testing.T) {
	observations := make([]StructuredCommandObservation, 0, 12)
	for i := 0; i < 12; i++ {
		stdout := strings.Repeat(fmt.Sprintf("bulk-%02d ", i), 9000)
		if i == 0 {
			stdout = "OLD_BULK_MARKER " + stdout
		}
		if i == 11 {
			stdout = "LATEST_OK " + stdout
		}
		observations = append(observations, StructuredCommandObservation{
			Step:     i + 1,
			Command:  fmt.Sprintf("command-%02d", i),
			ExitCode: 0,
			Stdout:   stdout,
		})
	}
	memories := []SessionMemory{{Kind: "documentation_brief", Content: strings.Repeat("large documentation brief ", 8000)}}
	req := buildStructuredCommandRequestWithContextRecipesSurveyAndPrep(
		"continue the app",
		nil,
		memories,
		observations,
		t.TempDir(),
		[]StructuredObjective{{ID: "implement_app", Description: "implement app", Status: "pending"}},
		MinimalContext{Summary: strings.Repeat("minimal context ", 4000)},
		nil,
		WorksiteSurvey{},
		PrepContextBundle{},
	)
	joined := structuredRequestMessagesText(req)
	if got := approxOllamaRequestChars(req); got > defaultStructuredPlannerPromptBudgetChars {
		t.Fatalf("request was not budgeted: got %d want <= %d", got, defaultStructuredPlannerPromptBudgetChars)
	}
	if strings.Contains(joined, "OLD_BULK_MARKER") {
		t.Fatalf("old bulky observation survived budget compaction")
	}
	for _, want := range []string{"context_compacted", "command-11", "LATEST_OK"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("budgeted request missing %q", want)
		}
	}
}

func TestShellSpecialistRequestBudgetsObservationContext(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Step: 1, Command: "old", ExitCode: 0, Stdout: "OLD_SHELL_BULK " + strings.Repeat("x", 20000)},
		{Step: 2, Command: "latest", ExitCode: 0, Stdout: "LATEST_SHELL_EVIDENCE " + strings.Repeat("y", 20000)},
	}
	req := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt:      "continue",
		ToolTask:        "write source files",
		Observations:    observations,
		SessionMemories: []SessionMemory{{Kind: "documentation_brief", Content: strings.Repeat("memory ", 5000)}},
	})
	joined := structuredRequestMessagesText(req)
	if strings.Contains(joined, strings.Repeat("x", 1000)) || strings.Contains(joined, strings.Repeat("y", 1000)) {
		t.Fatalf("shell specialist request retained huge observation output")
	}
	if !strings.Contains(joined, "LATEST_SHELL_EVIDENCE") {
		t.Fatalf("shell specialist request dropped latest evidence")
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

func TestValidateShellProposalAllowsInspectionForInspectionObjective(t *testing.T) {
	toolTask := "Active objective(s): inspect_empty_placeholder_files,remove_empty_placeholder_files,verify_app_with_build,verify_app_with_test. Required next behavior: inspect existing files before removing anything."
	for _, command := range []string{
		`find . -name "Clock.js" -empty -print`,
		`cd /tmp/project && find . -name '*.js' -o -name '*.jsx'`,
		`ls -la src`,
	} {
		if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
			t.Fatalf("inspection command %q should be allowed for inspection objective: %v", command, err)
		}
	}
}

func TestValidateShellProposalDoesNotTreatFindDeleteAsReadOnlyInspection(t *testing.T) {
	toolTask := "Active objective(s): inspect_empty_placeholder_files. Required next behavior: inspect existing files before removing anything."
	command := `find . -name "Clock.js" -empty -delete`
	if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
		t.Fatalf("substantive cleanup command should still be allowed as mutation: %v", err)
	}
	if structuredCommandLooksReadOnlyEvidence(command) {
		t.Fatalf("find -delete should not be classified as read-only evidence")
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskAllowsMutation(t *testing.T) {
	command := "cat > index.html <<'HTML'\n<div id=\"app\"></div>\nHTML"
	if err := validateShellProposalAgainstToolTask(command, "create or modify the actual project files now"); err != nil {
		t.Fatalf("mutation command rejected: %v", err)
	}
}

func TestValidateShellProposalAllowsInitialScaffoldSetupStep(t *testing.T) {
	command := "mkdir -p src/components && touch src/components/Note.js"
	if err := validateShellProposalAgainstToolTask(command, "setup note app component structure"); err != nil {
		t.Fatalf("initial scaffold setup step rejected: %v", err)
	}
}

func TestValidateShellProposalRejectsTouchForFocusedTDDFile(t *testing.T) {
	err := validateShellProposalAgainstToolTask(
		"touch src/App.test.js",
		"Create a focused failing test for the App component before implementation.",
	)
	if err == nil {
		t.Fatal("expected touch test file to be rejected for focused TDD work")
	}
	if !strings.Contains(err.Error(), "substantive source/build/test content") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateShellProposalRequiresNestedEmptyFileTarget(t *testing.T) {
	toolTask := "Recovery required. Completion is blocked because empty project files remain. Empty file(s): music-production-app/src/components/Sequencer.js,music-production-app/src/components/Track.js. Active task: build a React app."
	for _, command := range []string{
		`echo 'console.log("Hello")' > index.js`,
		`echo 'export default function App() { return null }' > src/App.js`,
		`echo 'import unittest' > tests/test_example.py`,
	} {
		err := validateShellProposalAgainstToolTask(command, toolTask)
		if err == nil {
			t.Fatalf("expected wrong-target command to be rejected: %s", command)
		}
	}
}

func TestValidateShellProposalAllowsNestedEmptyFileTarget(t *testing.T) {
	toolTask := "Recovery required. Completion is blocked because empty project files remain. Empty file(s): music-production-app/src/components/Sequencer.js,music-production-app/src/components/Track.js. Active task: build a React app."
	commands := []string{
		`cat > music-production-app/src/components/Sequencer.js <<'JS'
export default function Sequencer() { return null; }
JS`,
		`cd music-production-app && cat > src/components/Track.js <<'JS'
export default function Track() { return null; }
JS`,
	}
	for _, command := range commands {
		if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
			t.Fatalf("expected nested-target command to be allowed: %v\n%s", err, command)
		}
	}
}

func TestProgressionGateEmptyFileRecoveryWritesCodeOwnedPathWithoutShell(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "src", "Clock.js")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	decision := ProgressionDecision{
		Action:           ProgressForceRecovery,
		Reason:           "empty project files remain; deterministic empty-file recovery required",
		RecoveryToolTask: emptyProjectFilesRecoveryToolTask("Finish QA on this React app", nil, workspace),
	}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	handled, err := runProgressionGateRecovery(
		context.Background(),
		4,
		"Finish QA on this React app",
		decision,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected deterministic empty-file recovery to handle without shell specialist")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) == "" {
		t.Fatal("empty-file recovery left target empty")
	}
	appliedExactPath := false
	for _, obs := range result.Observations {
		if strings.HasPrefix(obs.Command, "empty_file.apply update src/Clock.js") {
			appliedExactPath = true
			break
		}
	}
	if !appliedExactPath {
		t.Fatalf("empty-file recovery did not apply exact queued file path: %#v", result.Observations)
	}
	if !structuredEventsContain(events, "empty_file_recovery_applied") {
		t.Fatalf("missing deterministic empty-file apply event: %#v", events)
	}
}

func TestArchitectFileWorkDoesNotFallThroughToShellPathSelection(t *testing.T) {
	workspace := t.TempDir()
	shell := &fakeShellCommandSpecialist{proposals: []ShellCommandProposal{{
		Command:   "cat src/components/UnqueuedPath.js",
		Rationale: "This should never be consulted for architect file work.",
	}}}
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Command: "architect.apply create package.json", ExitCode: 0},
		{Command: "architect.apply create vite.config.js", ExitCode: 0},
		{Command: "architect.apply create index.html", ExitCode: 0},
		{Command: "architect.apply create src/main.jsx", ExitCode: 0},
		{Command: "architect.apply create scripts/smoke-test.mjs", ExitCode: 0},
	}}
	events := []StructuredCommandEvent{}
	handled, err := runDelegatedShellSpecialist(
		context.Background(),
		3,
		"Build a React notes app",
		"Recovery required. Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace, ShellSpecialist: shell},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected architect file work to be handled before shell delegation")
	}
	if len(shell.inputs) != 0 {
		t.Fatalf("shell specialist was called for code-owned file work: %#v", shell.inputs)
	}
	if !structuredEventsContain(events, "architect_work_item_no_capable_actor") && !structuredEventsContain(events, "structured_tool_delegation_blocked_for_code_owned_file") {
		t.Fatalf("missing code-owned file work block event: %#v", events)
	}
}

func TestPlannerCommandPreemptedByArchitectCurrentItem(t *testing.T) {
	workspace := t.TempDir()
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{{
		Content:   `{"name":"music-studio","version":"0.1.0","type":"module","scripts":{"dev":"vite","build":"vite build","test":"node scripts/smoke-test.mjs","preview":"vite preview"},"dependencies":{"@vitejs/plugin-react":"latest","vite":"latest","react":"latest","react-dom":"latest"},"devDependencies":{}}`,
		Rationale: "valid package metadata",
	}}}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	handled, err := runArchitectLaneForCurrentItemBeforePlannerCommand(
		context.Background(),
		4,
		"Build a React music production studio app",
		"Implementation architect target root: . Create or modify the actual project files.",
		"close",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodeContentSpecialist:   code,
		},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected architect current item to preempt planner command")
	}
	if !structuredEventsContain(events, "planner_command_preempted_for_architect_item") {
		t.Fatalf("missing preemption event: %#v", events)
	}
	if !structuredEventsContain(events, "package_metadata_updated") {
		t.Fatalf("architect lane did not apply package metadata current item: %#v", events)
	}
	appliedPackage := false
	for _, obs := range result.Observations {
		if obs.Command == "architect.apply create package.json" || obs.Command == "architect.apply update package.json" {
			appliedPackage = true
		}
		if obs.Command == "close" || obs.RejectedCommand == "close" {
			t.Fatalf("preempted planner command was executed or rejected instead of bypassed: %#v", result.Observations)
		}
	}
	if !appliedPackage {
		t.Fatalf("architect lane did not apply package.json current item: %#v", result.Observations)
	}
	if len(code.inputs) > 0 && code.inputs[0].WorkItem.Path == "package.json" {
		t.Fatalf("deterministic package metadata handler should handle package.json before code specialist: %#v", code.inputs)
	}
}

func TestArchitectLaneReadsExistingFileBeforeUpdatingIt(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"scripts":{"test":"old","build":"old"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{{
		Content:   `{"name":"music-studio","version":"0.1.0","type":"module","scripts":{"dev":"vite","build":"vite build","test":"node scripts/smoke-test.mjs","preview":"vite preview"},"dependencies":{"@vitejs/plugin-react":"latest","vite":"latest","react":"latest","react-dom":"latest"},"devDependencies":{}}`,
		Rationale: "valid package metadata update",
	}}}
	result := CommandDecisionResult{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		2,
		"Build a React app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodeContentSpecialist:   code,
		},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		nil,
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected architect lane to handle read/update queue")
	}
	if len(result.Observations) < 2 || result.Observations[0].Command != "architect.read package.json" || result.Observations[1].Command != "architect.apply update package.json" {
		t.Fatalf("expected read then update observations, got %#v", result.Observations)
	}
	if len(code.inputs) > 0 && code.inputs[0].WorkItem.Path == "package.json" {
		t.Fatalf("deterministic package metadata handler should handle package.json before code specialist: %#v", code.inputs)
	}
	updated, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), `"test": "node scripts/smoke-test.mjs"`) {
		t.Fatalf("package metadata handler did not replace fake test script: %s", string(updated))
	}
}

func TestCodexNotConfiguredPackageMetadataRoutesToLocalHandler(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"name":"notes","version":"1.0.0","scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var nilCodex *CodexSDKArchitectAgent
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		5,
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodexArchitectAgent:     nilCodex,
		},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected package metadata handler to handle package.json work")
	}
	if !structuredEventsContain(events, "external_agent_unavailable") {
		t.Fatalf("missing external_agent_unavailable event: %#v", events)
	}
	if structuredEventsContain(events, "external_agent_started") || structuredEventsContain(events, "codex_sdk_architect_agent_started") {
		t.Fatalf("unconfigured external agent should not start: %#v", events)
	}
	if !structuredEventsContain(events, "package_metadata_updated") || !structuredEventsContain(events, "scripts_configured") || !structuredEventsContain(events, "package_json_valid") {
		t.Fatalf("missing package metadata evidence events: %#v", events)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "module"`, `"dev": "vite --host 0.0.0.0"`, `"build": "vite build"`, `"preview": "vite --host 0.0.0.0"`, `"@vitejs/plugin-react": "latest"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("package.json missing %q:\n%s", want, string(content))
		}
	}
}

func TestCodexNotConfiguredSourceFileWorkRoutesToLocalCodeSpecialist(t *testing.T) {
	workspace := t.TempDir()
	var nilCodex *CodexSDKArchitectAgent
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{{
		Content:   "import React, { useState } from 'react';\n\nexport default function App() {\n  const [level, setLevel] = useState(1);\n  return React.createElement('main', null,\n    React.createElement('h1', null, 'Notes'),\n    React.createElement('button', { type: 'button', onClick: () => setLevel(level + 1) }, 'Add note'),\n    React.createElement('input', { type: 'range', value: level, onChange: (event) => setLevel(Number(event.target.value)) })\n  );\n}\n",
		Rationale: "local source implementation",
	}}}
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Command: "architect.apply create package.json", ExitCode: 0},
		{Command: "architect.apply create vite.config.js", ExitCode: 0},
		{Command: "architect.apply create index.html", ExitCode: 0},
		{Command: "architect.apply create src/main.jsx", ExitCode: 0},
		{Command: "architect.apply create scripts/smoke-test.mjs", ExitCode: 0},
	}}
	events := []StructuredCommandEvent{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		6,
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CodexArchitectAgent:     nilCodex,
			CodeContentSpecialist:   code,
		},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected local code specialist to handle source file work")
	}
	if len(code.inputs) == 0 || code.inputs[0].WorkItem.Path != "src/App.js" {
		t.Fatalf("source file work did not route to local code specialist: %#v", code.inputs)
	}
	if structuredEventsContain(events, "external_agent_started") || structuredEventsContain(events, "codex_sdk_architect_agent_started") {
		t.Fatalf("unconfigured external agent should not start: %#v", events)
	}
}

func TestPackageMetadataCommandsAllowedForPackageWork(t *testing.T) {
	toolTask := "work_kind: package_metadata_update setup_react_package_metadata configure_package_scripts install_dependencies"
	for _, command := range []string{
		`npm install react react-dom vite @vitejs/plugin-react`,
		`npm pkg set scripts.dev="vite --host 0.0.0.0"`,
		`npm pkg set scripts.build="vite build"`,
		`npm pkg set scripts.preview="vite --host 0.0.0.0"`,
		`npm pkg set type=module`,
		`npm pkg delete scripts.test`,
	} {
		if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
			t.Fatalf("package metadata command %q should be allowed: %v", command, err)
		}
	}
}

func TestPackageMetadataDependencyScopeRejectsUnrequestedDependency(t *testing.T) {
	toolTask := "work_kind: package_metadata_update setup_react_package_metadata install_dependencies"
	err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		toolTask,
		"Add routing because many React apps commonly need navigation.",
	)
	if err == nil {
		t.Fatal("expected react-router-dom to be rejected for package metadata work")
	}
	ledger := []StructuredObjective{{
		ID:       "setup_react_package_metadata",
		Status:   "pending",
		Source:   structuredObjectiveSourceUserExplicit,
		Required: true,
		Packages: reactVitePackageMetadataDependencies(),
	}}
	if err := validateStructuredCommandForRun("npm install react-router-dom", nil, t.TempDir(), ledger); err == nil {
		t.Fatal("dependency scope validation allowed unrequested react-router-dom")
	}
}

func TestSourceFileWorkFailsWithCapabilityEvidenceWhenAllActorsUnavailable(t *testing.T) {
	workspace := t.TempDir()
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Command: "architect.apply create package.json", ExitCode: 0},
		{Command: "architect.apply create vite.config.js", ExitCode: 0},
		{Command: "architect.apply create index.html", ExitCode: 0},
		{Command: "architect.apply create src/main.jsx", ExitCode: 0},
		{Command: "architect.apply create scripts/smoke-test.mjs", ExitCode: 0},
	}}
	events := []StructuredCommandEvent{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		7,
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected source work to fail with capability evidence")
	}
	if !structuredEventsContain(events, "architect_work_item_no_capable_actor") {
		t.Fatalf("missing no-capable-actor event: %#v", events)
	}
	if got := result.Observations[len(result.Observations)-1].Stderr; !strings.Contains(got, "no capable actor configured") {
		t.Fatalf("missing capability evidence stderr: %q", got)
	}
}

func TestCursorArchitectAgentOwnsCodingTestingAndValidationDelegation(t *testing.T) {
	workspace := t.TempDir()
	cursor := &fakeCursorArchitectAgent{
		results: []CursorArchitectAgentResult{{Summary: "changed files and validated proofs", AgentID: "agent_1", RunID: "run_1"}},
		run: func(input CursorArchitectAgentInput) error {
			if input.ArchitectContract.TargetRoot != "." {
				t.Fatalf("unexpected target root: %#v", input.ArchitectContract)
			}
			if input.Packet.Mode != "implementation_only" {
				t.Fatalf("cursor packet mode = %q", input.Packet.Mode)
			}
			if len(input.Packet.EditSurface) == 0 || input.Packet.EditSurface[0] != "main.go" {
				t.Fatalf("cursor packet edit surface = %#v", input.Packet.EditSurface)
			}
			if !stringListContains(input.Packet.Forbidden, "do not claim objective completion; Omnidex will run proof commands and decide completion") {
				t.Fatalf("cursor packet missing completion authority guardrail: %#v", input.Packet.Forbidden)
			}
			return os.WriteFile(filepath.Join(input.Workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
		},
	}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	contract := ImplementationArchitectContract{
		Role:       "implementation_architect",
		TargetRoot: ".",
		EditSurface: []string{
			"main.go",
		},
		WorkQueue: []ArchitectWorkItem{{
			ID:          "write_main",
			Operation:   "create",
			CWD:         ".",
			Path:        "main.go",
			Description: "Create the CLI entrypoint",
		}},
		CurrentItem: &ArchitectWorkItem{
			ID:          "write_main",
			Operation:   "create",
			CWD:         ".",
			Path:        "main.go",
			Description: "Create the CLI entrypoint",
		},
	}
	handled, err := runCursorArchitectAgentLane(
		context.Background(),
		2,
		"Create a Go CLI",
		"Implementation architect target root: . Create or modify the actual project files.",
		contract,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
			CursorArchitectAgent:    cursor,
		},
		WorksiteSurvey{},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		&result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected cursor architect lane to handle the task")
	}
	if len(cursor.inputs) != 1 {
		t.Fatalf("cursor agent calls = %d, want 1", len(cursor.inputs))
	}
	if !structuredEventsContain(events, "cursor_sdk_architect_agent_started") || !structuredEventsContain(events, "cursor_sdk_architect_agent_completed") || !structuredEventsContain(events, "cursor_sdk_architect_validation_passed") {
		t.Fatalf("missing cursor architect events: %#v", events)
	}
	if !hasImplementationArchitectProgress(result.Observations) {
		t.Fatalf("cursor architect result did not record architect progress: %#v", result.Observations)
	}
	if result.Observations[0].EvidenceKind != "implementation" || result.Observations[0].GeneratedBy != "cursor_sdk" {
		t.Fatalf("cursor result should be implementation evidence only: %#v", result.Observations[0])
	}
}

func TestBuildCursorArchitectPromptUsesMissionPacket(t *testing.T) {
	input := CursorArchitectAgentInput{
		UserPrompt: "build a notes app",
		Packet: CursorImplementationPacket{
			Task:        "Implement CRUD notes behavior",
			Mode:        "implementation_only",
			Workspace:   "/tmp/project",
			TargetRoot:  ".",
			EditSurface: []string{"src/App.jsx"},
			Objectives:  []string{"create note", "delete note"},
			ProofContract: CursorPacketProofContract{
				Commands:           []string{"npm run build"},
				EvidencePredicates: []string{"command_passed:npm run build"},
			},
			Forbidden: []string{"do not add react-router-dom"},
		},
	}
	prompt := buildCursorArchitectPrompt(input)
	for _, want := range []string{
		`"cursor_packet"`,
		`"mode": "implementation_only"`,
		`"edit_surface"`,
		`"proof_contract"`,
		"implementation evidence only",
		"Omnidex decides completion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("cursor prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"architect_contract"`) {
		t.Fatalf("cursor prompt should use mission packet instead of raw architect contract:\n%s", prompt)
	}
}

func TestBuildCodexArchitectPromptUsesMissionPacketAndNoCompletionAuthority(t *testing.T) {
	input := CursorArchitectAgentInput{
		UserPrompt: "build a notes app",
		Packet: CursorImplementationPacket{
			Task:        "Implement CRUD notes behavior",
			Mode:        "implementation_only",
			Workspace:   "/tmp/project",
			TargetRoot:  ".",
			EditSurface: []string{"src/App.jsx"},
			Objectives:  []string{"create note", "delete note"},
			ProofContract: CursorPacketProofContract{
				Commands:           []string{"npm run build"},
				EvidencePredicates: []string{"command_passed:npm run build"},
			},
			Forbidden: []string{"do not add react-router-dom"},
		},
	}
	prompt := buildCodexArchitectPrompt(input)
	for _, want := range []string{
		`"codex_packet"`,
		`"mode": "implementation_only"`,
		`"edit_surface"`,
		`"proof_contract"`,
		"implementation evidence only",
		"Omnidex will run proof commands",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("codex prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"architect_contract"`) {
		t.Fatalf("codex prompt should use mission packet instead of raw architect contract:\n%s", prompt)
	}
}

func TestSelectedExternalArchitectAgentPrefersCodexWhenConfigured(t *testing.T) {
	cursor := &fakeCursorArchitectAgent{}
	codex := &fakeCursorArchitectAgent{}
	agent, name := selectedExternalArchitectAgent(structuredCommandDecisionRunConfig{
		CursorArchitectAgent: cursor,
		CodexArchitectAgent:  codex,
	})
	if agent != codex || name != "codex_sdk" {
		t.Fatalf("selected agent = %#v %q, want codex", agent, name)
	}
}

func TestExternalArchitectAgentsRequireExplicitEnvSelection(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "cursor-key")
	t.Setenv("CODEX_API_KEY", "codex-key")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OMNI_ENABLE_CURSOR_ARCHITECT", "true")
	t.Setenv("OMNI_ENABLE_CODEX_ARCHITECT", "true")
	t.Setenv("OMNI_DISABLE_CURSOR_ARCHITECT", "false")
	t.Setenv("OMNI_DISABLE_CODEX_ARCHITECT", "false")
	t.Setenv("OMNI_ARCHITECT_AGENT", "")
	if NewCursorSDKArchitectAgentFromEnv() != nil || NewCodexSDKArchitectAgentFromEnv() != nil {
		t.Fatal("external architect agents should stay disabled unless OMNI_ARCHITECT_AGENT selects one")
	}
	t.Setenv("OMNI_ARCHITECT_AGENT", "cursor")
	if NewCursorSDKArchitectAgentFromEnv() == nil {
		t.Fatal("cursor architect should be configured when selected and enabled")
	}
	if NewCodexSDKArchitectAgentFromEnv() != nil {
		t.Fatal("codex architect should not be configured when cursor is selected")
	}
	t.Setenv("OMNI_ARCHITECT_AGENT", "codex")
	if NewCodexSDKArchitectAgentFromEnv() == nil {
		t.Fatal("codex architect should be configured when selected and enabled")
	}
	if NewCursorSDKArchitectAgentFromEnv() != nil {
		t.Fatal("cursor architect should not be configured when codex is selected")
	}
}

func TestExternalArchitectAgentStreamsNormalizedEvents(t *testing.T) {
	agent := &fakeStreamingArchitectAgent{events: []AgentEvent{
		{Type: "started", Message: "started"},
		{Type: "command", Message: "running build", Command: "npm run build"},
		{Type: "file_change", Message: "changed files", Files: []string{"src/App.jsx"}},
		{Type: "completed", Message: "finished"},
	}}
	events := []StructuredCommandEvent{}
	result, err := runExternalArchitectAgentTask(
		context.Background(),
		agent,
		"codex_sdk",
		CursorArchitectAgentInput{
			Workspace: t.TempDir(),
			Packet: CursorImplementationPacket{
				Mode:        "implementation_only",
				EditSurface: []string{"src/App.jsx"},
			},
		},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "finished" {
		t.Fatalf("summary = %q", result.Summary)
	}
	for _, want := range []string{"external_agent_started", "external_agent_command", "external_agent_file_change", "external_agent_completed"} {
		if !structuredEventsContain(events, want) {
			t.Fatalf("missing %s in %#v", want, events)
		}
	}
	commandEvent := structuredEventOfTypeForTest(events, "external_agent_command")
	if commandEvent == nil || commandEvent.Details["command"] != "npm run build" {
		t.Fatalf("command event missing command detail: %#v", commandEvent)
	}
}

func TestHumanCorrectionCancelsAndRestartsExternalAgentWithRevisedPacket(t *testing.T) {
	active := &fakeExternalAgentSession{}
	provider := &fakeStreamingArchitectAgent{events: []AgentEvent{{Type: "started", Message: "restarted"}}}
	input := CursorArchitectAgentInput{
		Workspace: t.TempDir(),
		Packet: CursorImplementationPacket{
			Mode:            "implementation_only",
			EditSurface:     []string{"src/App.jsx", "src/App.css"},
			Forbidden:       []string{"do not create a sibling project"},
			PreparedContext: []string{"repo summary"},
		},
	}
	events, revised, err := restartExternalAgentSessionWithCorrection(context.Background(), active, provider, "codex_sdk", input, HumanCorrection{
		Message:               "Do not add routing. Keep it single-page.",
		Authority:             "user",
		ForbiddenDependencies: []string{"react-router-dom"},
		AllowedFiles:          []string{"src/App.jsx"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.cancelCount != 1 || active.cleanupCount != 1 {
		t.Fatalf("active session cancel/cleanup = %d/%d, want 1/1", active.cancelCount, active.cleanupCount)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("provider inputs = %d, want 1", len(provider.inputs))
	}
	if got := revised.Packet.EditSurface; len(got) != 1 || got[0] != "src/App.jsx" {
		t.Fatalf("edit surface = %#v, want corrected allowed files", got)
	}
	if !testStringSliceContains(revised.Packet.Forbidden, "do not add dependency: react-router-dom") {
		t.Fatalf("forbidden missing dependency correction: %#v", revised.Packet.Forbidden)
	}
	if !testStringSliceContainsSubstring(revised.Packet.PreparedContext, "human_correction[user]: Do not add routing. Keep it single-page.") {
		t.Fatalf("prepared context missing human correction: %#v", revised.Packet.PreparedContext)
	}
	got := resultFromExternalAgentEvents(events)
	if got.Summary != "restarted" {
		t.Fatalf("summary = %q, want restarted", got.Summary)
	}
}

func testStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func testStringSliceContainsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func TestBuildCodeContentSpecialistRequestIncludesAuthoritativeFileContract(t *testing.T) {
	contract := ImplementationArchitectContract{
		AcceptanceCriteria: []string{"channel rack", "mixer controls", "visual timeline"},
	}
	cases := []struct {
		name string
		item ArchitectWorkItem
		want []string
	}{
		{
			name: "package_json",
			item: ArchitectWorkItem{Operation: "update", Path: "package.json"},
			want: []string{`"role":"npm_package_manifest"`, `"language":"json"`, "comments", "javascript module source"},
		},
		{
			name: "smoke_test",
			item: ArchitectWorkItem{Operation: "update", Path: "scripts/smoke-test.mjs"},
			want: []string{`"role":"deterministic_acceptance_probe"`, `"language":"node_javascript_module"`, "readFileSync", "process.exit"},
		},
		{
			name: "app_component",
			item: ArchitectWorkItem{Operation: "update", Path: "src/App.js"},
			want: []string{`"role":"react_application_component"`, `"language":"javascript_or_jsx_module"`, "channel rack", "mixer", "timeline"},
		},
		{
			name: "stylesheet",
			item: ArchitectWorkItem{Operation: "update", Path: "src/App.css"},
			want: []string{`"role":"css_stylesheet"`, `"language":"css"`, ".studio-shell", "react component"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := buildCodeContentSpecialistRequest(CodeContentSpecialistInput{
				UserPrompt:        "build a music studio",
				ArchitectContract: contract,
				WorkItem:          tc.item,
			})
			text := structuredRequestMessagesText(req)
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Fatalf("request missing %q:\n%s", want, text)
				}
			}
			if !strings.Contains(text, "file_contract is authoritative") {
				t.Fatalf("request missing authoritative file contract rule:\n%s", text)
			}
		})
	}
}

func TestValidateShellProposalRequiresArchitectTargetRoot(t *testing.T) {
	toolTask := "Recovery required. Implementation architect target root: react-music-production. All source edits, package scripts, and verification commands for this app must run inside react-music-production or use paths under react-music-production/."
	if err := validateShellProposalAgainstToolTask(`cat > src/App.js <<'JS'
export default function App() { return null; }
JS`, toolTask); err == nil {
		t.Fatal("expected root-relative source edit to be rejected")
	}
	for _, command := range []string{
		`cd react-music-production && cat > src/App.js <<'JS'
export default function App() { return null; }
JS`,
		`cat > react-music-production/src/App.js <<'JS'
export default function App() { return null; }
JS`,
	} {
		if err := validateShellProposalAgainstToolTask(command, toolTask); err != nil {
			t.Fatalf("expected architect-targeted command to be allowed: %v", err)
		}
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskRejectsPlaceholderMutation(t *testing.T) {
	for _, command := range []string{"touch Clock.js", "mkdir -p src && touch src/Clock.js"} {
		err := validateShellProposalAgainstToolTask(command, "Required next behavior: create or modify the actual project files now. Do not create placeholder-only files with touch or empty mkdir scaffolds.")
		if err == nil {
			t.Fatalf("expected placeholder mutation %q to be rejected", command)
		}
		if !strings.Contains(err.Error(), "placeholder-only") {
			t.Fatalf("unexpected error for %q: %v", command, err)
		}
	}
}

func TestValidateShellProposalAgainstSourceImplementationRejectsDependencyInstall(t *testing.T) {
	toolTask := "Active objective(s): setup_note_app,create_note_app_structure,implement_crud_operations,store_notes_in_memory. Required next behavior: create or modify the actual project files now with substantive source/build/test files."
	for _, command := range []string{"npm install", "npm install react-router-dom", "pnpm add react-router-dom", "cargo add chess"} {
		err := validateShellProposalAgainstToolTask(command, toolTask)
		if err == nil {
			t.Fatalf("expected dependency install %q to be rejected for source implementation task", command)
		}
		if !strings.Contains(err.Error(), "source file implementation") {
			t.Fatalf("unexpected error for %q: %v", command, err)
		}
	}
}

func TestValidateShellProposalAllowsDependencyInstallWhenToolTaskRequiresDependencies(t *testing.T) {
	toolTask := "Active objective(s): install_dependencies. Required next behavior: install dependencies for the selected React project."
	if err := validateShellProposalAgainstToolTask("npm install react react-dom", toolTask); err != nil {
		t.Fatalf("dependency install should be allowed for dependency objective: %v", err)
	}
}

func TestValidateShellProposalPolicesDependencyInstallRationale(t *testing.T) {
	toolTask := "Active objective(s): install_dependencies. Required next behavior: install dependencies for the selected React project."
	err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		toolTask,
		"Installing react-router-dom will allow navigation between components, which is a common requirement in many React applications.",
	)
	if err == nil {
		t.Fatal("expected weak common-requirement rationale to be rejected")
	}
	if !strings.Contains(err.Error(), "without tool_task or evidence-backed rationale") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		"Install dependency react-router-dom for the requested routed React app.",
		"User requested routed navigation.",
	); err != nil {
		t.Fatalf("explicit tool task package should be allowed: %v", err)
	}
	if err := validateShellProposalAgainstToolTaskWithRationale(
		"npm install react-router-dom",
		toolTask,
		"Observed build error: Cannot find module react-router-dom imported by src/App.js.",
	); err != nil {
		t.Fatalf("evidence-backed rationale should be allowed: %v", err)
	}
}

func TestValidateShellProposalAgainstWriteRequiredToolTaskRejectsDocumentationDownload(t *testing.T) {
	err := validateShellProposalAgainstToolTask(
		"curl -s https://ziglang.org/documentation/master/ > zig_doc.html",
		"Required next behavior: create or modify the actual project files now with substantive source/build/test files.",
	)
	if err == nil {
		t.Fatal("expected documentation download to be rejected for source-write recovery")
	}
	if !strings.Contains(err.Error(), "documentation download") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeterministicGoReactCalculusRecoveryAppliesAfterWriteRequiredStall(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend", "calculus-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "frontend", "calculus-frontend", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "calculus-api", "go.mod"), []byte("module calculus-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frontend", "calculus-frontend", "package.json"), []byte(`{"scripts":{"test":"react-scripts test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	command := deterministicProgressionRecoveryCommand(
		"Continue the calculus app using Go for the backend and React for the frontend.",
		ProgressionDecision{
			Reason:           "workspace inspection has not produced app files; creation step is now required",
			RecoveryToolTask: "Required next behavior: create or modify the actual project files now. Do not continue with read-only inventory commands.",
		},
		dir,
	)
	if command == "" {
		t.Fatal("expected deterministic Go + React calculus recovery command")
	}
	for _, want := range []string{"set -e", "backend/calculus-api/calc.go", "frontend/calculus-frontend/src/App.js", "getAllByText('2x')", "make test", "make build"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q", want)
		}
	}
}

func TestDeterministicZigCLICalculatorRecoveryCommandWritesVerifiedProject(t *testing.T) {
	dir := t.TempDir()
	if !deterministicZigCLICalculatorRecoveryApplies(
		"build a zig cli calculator",
		"required next behavior: create or modify actual project files with substantive source",
		dir,
	) {
		t.Fatal("expected Zig CLI calculator recovery to apply")
	}
	command := deterministicZigCLICalculatorRecoveryCommand()
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("recovery command failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ZIG_CALCULATOR_SOURCE_VERIFIED") {
		t.Fatalf("missing verification marker: %s", output)
	}
	for _, rel := range []string{"build.zig", "src/main.zig", "README.md"} {
		if !fileHasContent(filepath.Join(dir, rel)) {
			t.Fatalf("missing generated file %s", rel)
		}
	}
}

func TestSourceVerificationCompletionSatisfiedForGeneratedZigProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.zig"), []byte("const std = @import(\"std\");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.zig"), []byte("pub fn main() void {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	latest := StructuredCommandObservation{
		Command:  "python3 - <<'PY'",
		ExitCode: 0,
		Stdout:   "ZIG_CALCULATOR_SOURCE_VERIFIED build.zig src/main.zig README.md",
	}
	if sourceVerificationCompletionSatisfied("Build a Zig CLI calculator application.", dir, latest) {
		t.Fatal("arbitrary stdout marker must not satisfy source verification completion")
	}
	runtimeOwned := StructuredCommandObservation{
		Command:           "runtime.source_verify zig",
		ExitCode:          0,
		EvidenceKind:      "source_verification",
		GeneratedBy:       "runtime",
		VerifierID:        "zig-source-verifier",
		CheckedFiles:      []string{"build.zig", "src/main.zig"},
		CheckedPredicates: []string{"file_nonempty:build.zig", "file_nonempty:src/main.zig"},
	}
	if !sourceVerificationCompletionSatisfied("Build a Zig CLI calculator application.", dir, runtimeOwned) {
		t.Fatal("expected runtime-owned structured source verification evidence to satisfy completion")
	}
}

func TestDeterministicRustOmnidexChessRecoveryCommandWritesVerifiedProject(t *testing.T) {
	dir := t.TempDir()
	if !deterministicRustOmnidexChessRecoveryApplies(
		"build a rust cli chess game against omnidex",
		"planner repeatedly failed to produce source-writing action for empty app workspace",
		dir,
	) {
		t.Fatal("expected Rust Omnidex chess recovery to apply")
	}
	command := deterministicRustOmnidexChessRecoveryCommand()
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("recovery command failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "RUST_OMNIDEX_CHESS_SOURCE_VERIFIED") {
		t.Fatalf("missing verification marker: %s", output)
	}
	if !strings.Contains(string(output), "running 7 tests") {
		t.Fatalf("expected generated Rust tests to include board rendering coverage: %s", output)
	}
	for _, rel := range []string{"Cargo.toml", "src/lib.rs", "src/main.rs", "README.md"} {
		if !fileHasContent(filepath.Join(dir, rel)) {
			t.Fatalf("missing generated file %s", rel)
		}
	}
	lib, err := os.ReadFile(filepath.Join(dir, "src", "lib.rs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lib), "pub fn render_board") || !strings.Contains(string(lib), "Side to move") {
		t.Fatalf("generated chess app missing human-readable board renderer")
	}
	mainSource, err := os.ReadFile(filepath.Join(dir, "src", "main.rs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSource), "render_board(&state.board)") {
		t.Fatalf("generated CLI still does not render the board")
	}
}

func TestDeterministicRustOmnidexChessBoardRepairAppliesToFenOnlyProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Cargo.toml":  "[package]\nname = \"omnidex_chess_cli\"\nversion = \"0.1.0\"\nedition = \"2024\"\n\n[dependencies]\nchess = \"3.2\"\n",
		"src/lib.rs":  "use chess::Board;\npub struct OmnidexProvider;\n",
		"src/main.rs": "fn main() { println!(\"{}\", chess::Board::default()); }\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !deterministicRustOmnidexChessBoardRepairApplies(
		"improve this rust omnidex chess cli because fen is not human-readable; add a terminal board",
		"repeated command failed to advance after read-only inspection; modify board rendering",
		dir,
	) {
		t.Fatal("expected Rust chess board repair recovery to apply")
	}
}

func TestDeterministicGoReactCalculusSmokeRepairFixesAmbiguousTestAndRootModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend", "calculus-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "frontend", "calculus-frontend", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(dir, "backend", "calculus-api", "go.mod"):                   "module calculus-api\n",
		filepath.Join(dir, "go.mod"):                                              "module calculus\n",
		filepath.Join(dir, "frontend", "calculus-frontend", "src", "App.test.js"): "expect(screen.getByText('2x')).toBeInTheDocument();",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	command := deterministicProgressionRecoveryCommand(
		"Continue the calculus app using Go for the backend and React for the frontend.",
		ProgressionDecision{
			Reason:           "verification failed and project scaffold already exists",
			RecoveryToolTask: "Required next behavior: create or modify project files and rerun tests.",
		},
		dir,
	)
	if command == "" {
		t.Fatal("expected deterministic smoke repair command")
	}
	for _, want := range []string{"getAllByText('2x')", "fs.rmSync('go.mod'", "make test", "make build"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command missing %q", want)
		}
	}
}

func TestValidateNestedGoModuleCommandScopeRejectsRootGoModInit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend", "calculus-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "calculus-api", "go.mod"), []byte("module calculus-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateStructuredCommandForRunWithSurvey("go mod init calculus", nil, dir, nil, WorksiteSurvey{})
	if err == nil {
		t.Fatal("expected root go mod init to be rejected when nested module exists")
	}
	if !strings.Contains(err.Error(), "nested module") {
		t.Fatalf("unexpected error: %v", err)
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

func TestReconcileObjectiveLedgerDoesNotSatisfyAllVerificationFromIrrelevantGoTest(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "verify_backend_tests", Description: "Verify backend Go tests.", Status: "pending"},
		{ID: "verify_frontend_tests", Description: "Verify frontend tests.", Status: "pending"},
		{ID: "run_smoke_test", Description: "Run smoke test.", Status: "pending"},
		{ID: "verify_frontend_build", Description: "Verify frontend build.", Status: "pending"},
	}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "go test ./...",
		ExitCode: 0,
		Stdout:   "?   \tcalculus/frontend/calculus-frontend/node_modules/flatted/golang/pkg/flatted\t[no test files]",
	}, nil)

	for _, objective := range updated {
		if structuredObjectiveSatisfied(objective) {
			t.Fatalf("objective %s should not be satisfied by irrelevant root go test output: %#v", objective.ID, updated)
		}
	}
}

func TestReconcileObjectiveLedgerSatisfiesSpecificGoReactVerificationEvidence(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "verify_backend_tests", Description: "Verify backend Go tests.", Status: "pending"},
		{ID: "verify_frontend_tests", Description: "Verify frontend tests.", Status: "pending"},
		{ID: "run_smoke_test", Description: "Run smoke test.", Status: "pending"},
		{ID: "verify_frontend_build", Description: "Verify frontend build.", Status: "pending"},
	}
	updated := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "make test && make build",
		ExitCode: 0,
		Stdout:   "cd backend/calculus-api && go test ./...\nok  \tcalculus-api\t0.002s\nreact-scripts test --watchAll=false\nPASS src/App.test.js\ngo react calculus smoke test passed\nCompiled successfully.",
	}, nil)

	for _, objective := range updated {
		if !structuredObjectiveSatisfied(objective) {
			t.Fatalf("objective %s should be satisfied by make verification evidence: %#v", objective.ID, updated)
		}
	}
}

func TestReconcileObjectiveLedgerRequiresDockerLifecycleEvidence(t *testing.T) {
	ledger := []StructuredObjective{
		{ID: "create_dockerfile", Description: "Create Dockerfile", Status: "pending"},
		{ID: "build_docker_image", Description: "Build Docker image", Status: "pending"},
		{ID: "run_application_in_docker_container", Description: "Run application in Docker container", Status: "pending"},
	}
	afterDockerfile := reconcileStructuredObjectiveLedgerFromObservation(1, ledger, StructuredCommandObservation{
		Step:     1,
		Command:  "echo 'FROM nginx:alpine' > Dockerfile",
		ExitCode: 0,
		Stdout:   "Dockerfile created successfully.",
	}, nil)
	for _, objective := range afterDockerfile {
		if objective.ID != "create_dockerfile" && structuredObjectiveSatisfied(objective) {
			t.Fatalf("Dockerfile-only command should not satisfy lifecycle objective %s: %#v", objective.ID, afterDockerfile)
		}
	}

	afterLifecycle := reconcileStructuredObjectiveLedgerFromObservation(2, afterDockerfile, StructuredCommandObservation{
		Step:     2,
		Command:  "docker build -t app:test . && docker run -d --name app-test --restart=no -p 127.0.0.1:8080:80 app:test && curl -fsS http://127.0.0.1:8080/health && docker inspect -f '{{.State.Running}} {{.State.Restarting}} {{.RestartCount}}' app-test && docker logs app-test",
		ExitCode: 0,
		Stdout:   "Successfully built abc123\nrunning=true restarting=false restart_count=0\nhealth=ok\nDOCKER_LOGS_CLEAR",
	}, nil)
	for _, id := range []string{"build_docker_image", "run_application_in_docker_container"} {
		found := false
		for _, objective := range afterLifecycle {
			if objective.ID == id {
				found = true
				if !structuredObjectiveSatisfied(objective) {
					t.Fatalf("%s should be satisfied by lifecycle evidence: %#v", id, afterLifecycle)
				}
			}
		}
		if !found {
			t.Fatalf("missing objective %s", id)
		}
	}
}

func TestStructuredCommandDecisionAcceptsPartialCompletionAndContinues(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'build passed\n' && true # npm run build","done":false,"answer":""}`,
		`{"command":"printf 'test passed\n' && true # npm test","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"build and test passed"}`,
	}}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify build", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
			{ID: "verify_test", Description: "Verify test", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		},
	}}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   false,
		Reason: "build passed but tests remain",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify build", Status: "satisfied", Evidence: "npm run build exited 0"},
			{ID: "verify_test", Description: "Verify test", Status: "pending"},
		},
	}, {
		Done:   true,
		Reason: "build and test passed",
		ObjectiveLedger: []StructuredObjective{
			{ID: "verify_build", Description: "Verify build", Status: "satisfied", Evidence: "npm run build exited 0"},
			{ID: "verify_test", Description: "Verify test", Status: "satisfied", Evidence: "npm test exited 0"},
		},
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"verify app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		structuredCommandDecisionRunConfig{
			PromptInterpreter: interpreter,
			CompletionChecker: checker,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Command != "printf 'test passed\n' && true # npm test" {
		t.Fatalf("final command = %q, want continued test command", result.Command)
	}
	if len(checker.inputs) == 0 {
		t.Fatal("completion checker should run after both queue items pass")
	}
	for _, input := range checker.inputs {
		if pending := pendingStructuredObjectives(input.ObjectiveLedger); len(pending) != 0 {
			t.Fatalf("completion checker received pending work before queue exhaustion: %#v", pending)
		}
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("ledger still pending: %#v", result.ObjectiveLedger)
	}
}

func TestCompletionCheckerCannotSatisfyObjectiveWithoutDeterministicEvidence(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "verify_build",
		Description:      "Verify build",
		Status:           "pending",
		Source:           structuredObjectiveSourceUserExplicit,
		Required:         true,
		RequiredEvidence: []string{"command_passed:npm run build"},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "looks complete",
		ObjectiveLedger: []StructuredObjective{{
			ID:       "verify_build",
			Status:   "satisfied",
			Evidence: "validator said build passed",
		}},
	}}}
	events := []StructuredCommandEvent{}
	result := runCompletionCheckDetailed(
		context.Background(),
		3,
		"build the app",
		t.TempDir(),
		ledger,
		MinimalContext{},
		nil,
		"done",
		checker,
		WorksiteSurvey{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
	)
	if result.Accepted {
		t.Fatal("completion checker must not accept objective claims without deterministic evidence")
	}
	if pending := pendingStructuredObjectives(result.Ledger); len(pending) != 1 {
		t.Fatalf("objective should remain pending, got %#v", result.Ledger)
	}
	if !structuredEventsContain(events, "completion_check_claim_rejected_for_missing_evidence") {
		t.Fatalf("missing claim rejection event: %#v", events)
	}
}

func TestCompletionCheckerClaimAcceptedWhenRequiredEvidencePassed(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:               "verify_build",
		Description:      "Verify build",
		Status:           "pending",
		Source:           structuredObjectiveSourceUserExplicit,
		Required:         true,
		RequiredEvidence: []string{"command_passed:npm run build"},
	}}
	checker := &fakeCompletionChecker{checks: []CompletionCheck{{
		Done:   true,
		Reason: "build evidence exists",
		ObjectiveLedger: []StructuredObjective{{
			ID:       "verify_build",
			Status:   "satisfied",
			Evidence: "npm run build exited 0",
		}},
	}}}
	result := runCompletionCheckDetailed(
		context.Background(),
		3,
		"build the app",
		t.TempDir(),
		ledger,
		MinimalContext{},
		[]StructuredCommandObservation{{Command: "npm run build", ExitCode: 0, Stdout: "built in 1s"}},
		"done",
		checker,
		WorksiteSurvey{},
		nil,
	)
	if !result.Accepted {
		t.Fatalf("completion checker claim should be accepted after required evidence passed: %#v", result.Ledger)
	}
}

func quoteJSONForTest(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(value) + `"`
}

func TestStructuredCommandRequestIncludesTypedWorkQueue(t *testing.T) {
	req := buildStructuredCommandRequestWithContextRecipesSurveyAndPrepRaw(
		"build a React notes app",
		nil,
		nil,
		nil,
		t.TempDir(),
		[]StructuredObjective{{
			ID:          "complete_notes_app",
			Description: "Complete notes app implementation",
			Status:      "pending",
			Kind:        string(WorkItemKindArchitect),
			Source:      structuredObjectiveSourceUserExplicit,
			Required:    true,
		}},
		MinimalContext{},
		nil,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		PrepContextBundle{},
	)
	activeTask := activeTaskJSONForTest(t, req.Messages[len(req.Messages)-1].Content)
	for _, want := range []string{`"work_items"`, `"current_work_item"`, `"kind":"architect"`, `"kind":"create"`, `"scope"`, `"paths":["package.json"]`} {
		if !strings.Contains(activeTask, want) {
			t.Fatalf("active task missing %q: %s", want, activeTask)
		}
	}
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
