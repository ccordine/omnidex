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

func TestStructuredCommandDecisionStopsWhenEvaluatorFails(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "structured response evaluator failed") || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("error = %v, want explicit evaluator failure", err)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want one failed boundary call", len(evaluator.inputs))
	}
	if result.Answer != "" {
		t.Fatalf("evaluator failure produced successful answer %q", result.Answer)
	}
	if !structuredEventsContain(events, "structured_response_evaluator_failed") {
		t.Fatalf("missing evaluator failure event: %#v", events)
	}
}

func TestStructuredCommandDecisionStopsWhenEvaluatorScoringIsInconsistent(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "inconsistent") {
		t.Fatalf("error = %v, want explicit inconsistent-evaluator failure", err)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("evaluator calls = %d, want one inconsistent boundary call", len(evaluator.inputs))
	}
	if result.Answer != "" {
		t.Fatalf("inconsistent evaluator produced successful answer %q", result.Answer)
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
	substantive := "cat > src/App.js <<'EOF'\nexport default function App(){ return 'notes memory crud'; }\nEOF"
	memoryWrite := "printf '\\n// memory state managed with useState\\n' >> src/App.js"
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"echo 'Step 1: Set up Notes Context' && echo 'Step 2: Update App.js' && echo 'Step 3: Run the Application'","done":false,"answer":"","objective_ledger":[{"id":"create_note_taking_component","description":"Create note taking component UI","status":"pending","source":"user_explicit","required":true},{"id":"implement_memory_state_management","description":"Implement memory state management","status":"pending","source":"user_explicit","required":true}]}`,
		`{"command":` + quoteJSONForTest(substantive) + `,"done":false,"answer":"","objective_ledger":[{"id":"create_note_taking_component","description":"Create note taking component UI","status":"satisfied","evidence":"src/App.js written"}]}`,
		`{"command":` + quoteJSONForTest(memoryWrite) + `,"done":false,"answer":"","objective_ledger":[{"id":"implement_memory_state_management","description":"Implement memory state management","status":"satisfied","evidence":"App contains memory marker"}]}`,
		`{"command":"grep -q 'memory state managed with useState' src/App.js","done":false,"answer":""}`,
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
	command := "mkdir -p src/components src/pages src/hooks"
	err := validateStructuredCommandForRun(command, nil, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("initial directory scaffold should be allowed: %v", err)
	}
	ledger := []StructuredObjective{
		{ID: "setup_note_app", Description: "Set up the note-taking app", Status: "pending"},
		{ID: "implement_crud_operations", Description: "Implement CRUD operations", Status: "pending"},
	}
	err = validateStructuredCommandForRun("touch src/Another.js", []StructuredCommandObservation{{Step: 1, Command: command, ExitCode: 0}}, t.TempDir(), ledger)
	if err == nil {
		t.Fatal("expected touch of source file to be rejected")
	}
	if !strings.Contains(err.Error(), "empty source files") && !strings.Contains(err.Error(), "placeholder-only") {
		t.Fatalf("expected touch rejection, got %v", err)
	}
	substantive := "printf %s 'export default function App(){ return \"Notes\"; }' > src/App.js"
	if err := validateStructuredCommandForRun(substantive, nil, t.TempDir(), ledger); err != nil {
		t.Fatalf("substantive app write should be allowed: %v", err)
	}
}

func TestSemicolonMutationChainIgnoresQuotedAndHeredocContent(t *testing.T) {
	for _, command := range []string{
		`printf %s 'export default function App(){ return "Notes"; }' > src/App.js`,
		"cat > src/App.js <<'JS'\nexport default function App(){ return 'Notes'; }\nJS",
	} {
		if semicolonMutationChain(command) {
			t.Fatalf("content semicolon was treated as a command separator: %q", command)
		}
	}
	if !semicolonMutationChain("printf notes > src/App.js; npm test") {
		t.Fatal("real semicolon-chained mutation and verification was not rejected")
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

func TestRepeatedSuccessfulBuildAllowedAfterMutation(t *testing.T) {
	command := "npm run build"
	observations := []StructuredCommandObservation{
		{Step: 1, Command: command, ExitCode: 0, Stdout: "built in 1s"},
		{Step: 2, Command: "architect.apply update src/App.jsx", EvidenceKind: "implementation", ExitCode: 0, Stdout: "updated source"},
	}
	if repeatedSuccessfulStructuredCommand(command, observations) {
		t.Fatal("build verifier rerun should be allowed after workspace mutation")
	}
}

func TestRepeatedReadOnlyCommandWithNoStateChangeUsesPriorEvidence(t *testing.T) {
	command := "find . -maxdepth 1 -type f"
	observations := []StructuredCommandObservation{{Step: 1, Command: command, ExitCode: 0, Stdout: "./package.json\n"}}
	if !repeatedSuccessfulStructuredCommand(command, observations) {
		t.Fatal("read-only repeated command without state change should use prior evidence")
	}
}

func TestDependencyInstallEventEmittedOnlyAfterInstallCommandRuns(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho fake npm install \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := fmt.Sprintf("PATH=%s:$PATH npm install react", shellQuote(binDir))
	events := []StructuredCommandEvent{}
	result := CommandDecisionResult{}
	if err := runStructuredPayloadCommand(context.Background(), 1, command, workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, func(evt StructuredCommandEvent) {
		events = append(events, evt)
	}, &result); err != nil {
		t.Fatal(err)
	}
	if !structuredEventsContain(events, "dependencies_installed") {
		t.Fatalf("missing dependencies_installed after successful npm install command: %#v", events)
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
	if err := os.MkdirAll(filepath.Join(workspace, "src", "components"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := `printf 'export default function Calculator(){ return null; }\n' > src/components/Calculator.jsx`
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":"` + command + `","done":false,"answer":""}`,
		`{"command":"grep -q Calculator src/components/Calculator.jsx","done":false,"answer":""}`,
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
