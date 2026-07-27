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
)

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

func TestStructuredLoopStateResetsAfterSuccessfulCommand(t *testing.T) {
	command := "npm install @hotwired/stimulus recyclr tailwindcss webpack webpack-cli --save-dev"
	observations := []StructuredCommandObservation{
		{Step: 1, Command: command, ExitCode: 1, Stderr: "npm failed"},
		{Step: 2, RejectedCommand: command, ExitCode: 1, Stderr: "anti_loop: command rejected again after prior failure/rejection count=2"},
		{Step: 3, Command: "find . -maxdepth 2 -type f | sort", ExitCode: 0, Stdout: "./package.json\n./src/App.jsx\n"},
	}

	state := structuredLoopStateFromState([]StructuredObjective{{ID: "implement_calculator_ui", Status: "pending"}}, observations)
	if state.Status != "progressing" {
		t.Fatalf("status = %q, want progressing; state=%#v", state.Status, state)
	}
	if state.RepeatKind != "" || state.RepeatedCommand != "" || state.LastBlocker != "" {
		t.Fatalf("successful latest command should clear stale loop context: %#v", state)
	}
	if !strings.Contains(state.Instruction, "Latest command completed successfully") {
		t.Fatalf("instruction = %q", state.Instruction)
	}
}

func TestStructuredLoopStateResetsAfterSuccessfulCommandFollowingDoneRejection(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Step: 1, ExitCode: 1, Stderr: "done rejected: pending objective(s) remain: implement_calculator_logic; run command(s) that satisfy the objective ledger before finishing"},
		{Step: 2, ExitCode: 1, Stderr: "done rejected: pending objective(s) remain: implement_calculator_logic; run command(s) that satisfy the objective ledger before finishing"},
		{Step: 3, Command: "npm run build", ExitCode: 0, Stdout: "built"},
	}

	state := structuredLoopStateFromState([]StructuredObjective{{ID: "implement_calculator_logic", Status: "pending"}}, observations)
	if state.Status != "progressing" {
		t.Fatalf("status = %q, want progressing; state=%#v", state.Status, state)
	}
	if state.RepeatKind != "" || state.LastBlocker != "" {
		t.Fatalf("successful latest command should clear stale done rejection context: %#v", state)
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

func TestOllamaPromptInterpreterRejectsMalformedResponseWithoutFallback(t *testing.T) {
	client := &fakeCommandDecisionClient{responses: []string{"not-json"}}
	interpreter := NewOllamaPromptInterpreter(client)
	interpretation, err := interpreter.InterpretPrompt(context.Background(), PromptInterpretationInput{
		UserPrompt:     "Build a React JS music production app",
		WorksiteSurvey: WorksiteSurvey{ProjectState: projectStateEmptyDirectory},
	})
	if err == nil {
		t.Fatalf("malformed response produced fallback interpretation: %#v", interpretation)
	}
	if len(interpretation.ObjectiveLedger) != 0 {
		t.Fatalf("malformed response produced objectives: %#v", interpretation.ObjectiveLedger)
	}
}

func TestStructuredCommandDecisionStopsWhenPromptInterpreterFails(t *testing.T) {
	client := &fakeCommandDecisionClient{}
	interpreter := &fakePromptInterpreter{errors: []error{errors.New("interpreter unavailable")}}
	events := []StructuredCommandEvent{}
	_, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"build the app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(event StructuredCommandEvent) { events = append(events, event) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: t.TempDir(),
			PromptInterpreter:       interpreter,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "prompt interpreter failed") || !strings.Contains(err.Error(), "interpreter unavailable") {
		t.Fatalf("error = %v, want explicit interpreter failure", err)
	}
	if client.calls != 0 {
		t.Fatalf("planner called after interpreter failure: %d", client.calls)
	}
	if !structuredEventsContain(events, "prompt_interpreter_failed") {
		t.Fatalf("missing prompt-interpreter failure event: %#v", events)
	}
}

func TestStructuredCommandDecisionStopsWhenContextSummarizerFails(t *testing.T) {
	client := &fakeCommandDecisionClient{}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		UserOperation: userOperationFixExisting,
		ObjectiveLedger: []StructuredObjective{{
			ID:          "fix_existing_app",
			Description: "Fix the existing app",
			Status:      "pending",
			Kind:        string(WorkItemKindUpdate),
			Source:      structuredObjectiveSourceUserExplicit,
			Required:    true,
		}},
	}}}
	summarizer := &fakeContextSummarizer{errors: []error{errors.New("summary unavailable")}}
	events := []StructuredCommandEvent{}
	_, err := runStructuredCommandDecisionWithConfig(
		context.Background(),
		"fix the existing app",
		nil,
		client,
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(event StructuredCommandEvent) { events = append(events, event) },
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: t.TempDir(),
			PromptInterpreter:       interpreter,
			ContextSummarizer:       summarizer,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "context summarizer failed") || !strings.Contains(err.Error(), "summary unavailable") {
		t.Fatalf("error = %v, want explicit summarizer failure", err)
	}
	if client.calls != 0 {
		t.Fatalf("planner called after context summarizer failure: %d", client.calls)
	}
	if !structuredEventsContain(events, "minimal_context_failed") {
		t.Fatalf("missing context-summarizer failure event: %#v", events)
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

func TestStructuredPlannerAndShellRequireDocumentationAndRealToolchainEvidence(t *testing.T) {
	plannerReq := buildStructuredCommandRequest("Build a Zig CLI calculator.", nil, nil)
	plannerContent := strings.ToLower(plannerReq.ContextSystem + "\n" + joinOllamaMessageContent(plannerReq.Messages))
	for _, want := range []string{"unfamiliar language", "official docs", "smallest hello-world project", "must leave compiler, build, and test objectives blocked", "tool=patch.apply"} {
		if !strings.Contains(plannerContent, want) {
			t.Fatalf("planner request missing %q: %s", want, plannerContent)
		}
	}

	shellReq := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt: "Build a Zig CLI calculator.",
		ToolTask:   "Required next behavior: create or modify the actual project files now for an unfamiliar language/toolchain.",
	})
	shellContent := strings.ToLower(joinOllamaMessageContent(shellReq.Messages))
	for _, want := range []string{"official documentation", "installed tool help", "substantive source/build/test files", "must not satisfy compiler, build, or test objectives"} {
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
		`{"command":"pwd","done":false,"answer":""}`,
		`{"command":"touch app.marker","done":false,"answer":""}`,
		`{"command":"ls app.marker","done":false,"answer":""}`,
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
