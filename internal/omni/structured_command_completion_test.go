package omni

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	updated, accepted, err := runCompletionCheck(
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
	if err != nil {
		t.Fatal(err)
	}
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
	installOfflineNPMArchitectStub(t)
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
		{Content: `<!doctype html><html><head><meta charset="UTF-8"><title>Music Studio</title></head><body><div id="root"></div><script type="module" src="/src/main.jsx"></script></body></html>` + "\n", Rationale: "html shell"},
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
	inputPaths := make([]string, 0, len(code.inputs))
	smokeIndex := -1
	appIndex := -1
	for i, input := range code.inputs {
		inputPaths = append(inputPaths, input.WorkItem.Path)
		switch input.WorkItem.Path {
		case "scripts/smoke-test.mjs":
			smokeIndex = i
		case "src/App.js":
			appIndex = i
		}
	}
	if smokeIndex < 0 || appIndex < 0 {
		t.Fatalf("architect queue missing smoke test or App.js: paths=%#v", inputPaths)
	}
	if !code.inputs[smokeIndex].TestFirst {
		t.Fatalf("smoke probe was not marked test-first: index=%d paths=%#v", smokeIndex, inputPaths)
	}
	if code.inputs[appIndex].TestFirst {
		t.Fatalf("App.js implementation was incorrectly marked test-first: index=%d paths=%#v", appIndex, inputPaths)
	}
	if smokeIndex >= appIndex {
		t.Fatalf("architect wrote implementation before its smoke test: smoke_index=%d app_index=%d paths=%#v", smokeIndex, appIndex, inputPaths)
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

func installOfflineNPMArchitectStub(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  install)
    exit 0
    ;;
  test)
    exec node scripts/smoke-test.mjs
    ;;
  run)
    if [ "$2" = "build" ]; then
      test -s src/App.js && test -s src/App.css && test -s src/main.jsx && test -s index.html
      exit $?
    fi
    ;;
esac
exit 64
`
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	if architectWorkItemSatisfied(item, workspace, ImplementationArchitectContract{}, nil) {
		t.Fatal("pre-existing file content must not satisfy an architect update item without current-run evidence")
	}
	if !architectWorkItemSatisfied(item, workspace, ImplementationArchitectContract{}, []StructuredCommandObservation{{
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

func TestGoBuildCountsAsPostWriteValidation(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Command: "cat > main.go <<'GO'\npackage main\nfunc main() {}\nGO", ExitCode: 0},
		{Command: "go build -o app .", ExitCode: 0},
	}
	ledger := []StructuredObjective{{
		ID:          "go_app",
		Description: "Build the Go application",
		Status:      "satisfied",
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
	}}
	if structuredCompletionNeedsPostWriteValidation("create and build a Go application", ledger, observations) {
		t.Fatal("go build after a source write should satisfy post-write validation")
	}
}
