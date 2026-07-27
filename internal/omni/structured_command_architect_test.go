package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		`{"command":"","done":true,"answer":"React music production app implemented and verified","objective_ledger":[{"id":"react_music_app","description":"Build the React music production app","status":"satisfied","source":"user_explicit","required":true,"evidence":"architect applied test, implementation, style, and npm run build passed"}]}`,
	}}
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{Content: "import { defineConfig } from 'vite';\nimport react from '@vitejs/plugin-react';\nexport default defineConfig({ plugins: [react()] });\n", Rationale: "vite config"},
		{Content: `<!doctype html><html><head><meta charset="UTF-8"><title>Music Studio</title></head><body><div id="root"></div><script type="module" src="/src/main.jsx"></script></body></html>` + "\n", Rationale: "html shell"},
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
		"source_probe_boundary",
		"completion_gate",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("active task message missing TDD policy %q: %s", want, message)
		}
	}
}

func TestStructuredCommandDecisionRejectsEmptyInterpreterLedger(t *testing.T) {
	client := &fakeCommandDecisionClient{}
	interpreter := &fakePromptInterpreter{interpretations: []PromptInterpretation{{
		UserOperation: userOperationFixExisting,
	}}}
	events := []StructuredCommandEvent{}
	result, err := runStructuredCommandDecisionWithConfig(
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
		},
	)
	if err == nil || !strings.Contains(err.Error(), "returned no executable objectives") {
		t.Fatalf("error = %v, want explicit empty-ledger failure", err)
	}
	if client.calls != 0 {
		t.Fatalf("planner called after invalid interpreter ledger: %d", client.calls)
	}
	if result.Command != "" || len(result.Observations) != 0 {
		t.Fatalf("invalid interpreter ledger produced execution state: %#v", result)
	}
	if !structuredEventsContain(events, "prompt_interpreter_invalid") {
		t.Fatalf("missing invalid-interpreter event: %#v", events)
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

func TestAbstractReactPlaybookUsedWithoutProjectIdentityLeak(t *testing.T) {
	memories := []SessionMemory{{
		Kind: validatedPlaybookKind,
		Content: `{
			"name": "fruityloops_react",
			"task_pattern": "Vite React notes CRUD app",
			"command_sequence": ["cat <<'EOF' > src/App.jsx\nprior app\nEOF", "npm run build"],
			"validation_signals": ["npm run build"],
			"confidence": 90,
			"scope_policy": "advisory_only"
		}`,
		Tags: []string{"validated-playbook", "procedure-memory", "advisory-only", "react", "vite"},
	}}
	req := buildStructuredCommandRequestWithMemoriesAndCWD("build an unrelated React notes app", nil, memories, nil, t.TempDir())
	joined := joinOllamaMessageContent(req.Messages)
	if strings.Contains(strings.ToLower(joined), "fruityloops") {
		t.Fatalf("playbook leaked old project identity: %s", joined)
	}
	if !strings.Contains(joined, "Vite React notes CRUD app") {
		t.Fatalf("abstract playbook pattern missing: %s", joined)
	}
	if !strings.Contains(joined, "may-create-scope:false") || !strings.Contains(joined, "advisory-only") {
		t.Fatalf("memory authority labels missing: %s", joined)
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
