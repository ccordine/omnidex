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
	for _, polluted := range []string{"Pattaya", "Saipan", "wttr.in", "news.google.com", "Build a React TypeScript app."} {
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
		ObjectiveLedger: []StructuredObjective{requiredCommandObjectiveForTest(
			"report_standalone_scope",
			"Report evidence for the standalone prompt without reference history",
			"printf",
		)},
	}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary:   "Create only the requested React project.",
		OpenItems: []string{"React project"},
	}}}
	history := []Message{
		{Role: "user", Content: "Create a Stimulus Tailwind RecyclrJS webpack calculator."},
		{Role: "assistant", Content: "Installed @hotwired/stimulus tailwindcss recyclr-js webpack."},
	}

	result, err := runStructuredCommandDecisionWithConfig(
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
		t.Fatalf("%v; result=%#v", err, result)
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
		"install the dependencies for this existing npm project",
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
		"install the dependencies for this existing npm project",
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
		"install the dependencies for this existing npm project",
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

func observationsContainCommand(observations []StructuredCommandObservation, want string) bool {
	normalizedWant := normalizeStructuredCommandForComparison(want)
	for _, observation := range observations {
		if normalizeStructuredCommandForComparison(observation.Command) == normalizedWant {
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
