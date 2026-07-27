package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	command := "mkdir -p src/components"
	if err := validateShellProposalAgainstToolTask(command, "setup note app component structure"); err != nil {
		t.Fatalf("initial scaffold setup step rejected: %v", err)
	}
	if err := validateShellProposalAgainstToolTask("touch src/components/Note.js", "setup note app component structure"); err == nil {
		t.Fatal("touch source file should be rejected even during scaffold setup")
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
	if !strings.Contains(err.Error(), "empty source files") && !strings.Contains(err.Error(), "substantive source/build/test content") {
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

func TestProgressionGateEmptyFileRecoveryFailsWithoutCodeSpecialist(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "code content specialist is required") {
		t.Fatalf("error = %v, want explicit missing-specialist failure", err)
	}
	if !handled {
		t.Fatal("empty-file recovery must own and report its failure")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 0 {
		t.Fatalf("missing specialist wrote fallback content: %q", content)
	}
	if !structuredEventsContain(events, "empty_file_recovery_failed") {
		t.Fatalf("missing explicit empty-file failure event: %#v", events)
	}
	if structuredEventsContain(events, "empty_file_recovery_applied") {
		t.Fatalf("missing specialist emitted forbidden apply event: %#v", events)
	}
}

func TestProgressionGateEmptyFileRecoveryWritesValidatedSpecialistContent(t *testing.T) {
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
		Reason:           "empty project files remain; empty-file recovery required",
		RecoveryToolTask: emptyProjectFilesRecoveryToolTask("Finish QA on this React app", nil, workspace),
	}
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{{
		Content:   "export default function Clock() { return <time>12:00</time>; }\n",
		Rationale: "Implement the queued Clock component.",
	}}}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	handled, err := runProgressionGateRecovery(
		context.Background(),
		4,
		"Finish QA on this React app",
		decision,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace, CodeContentSpecialist: code},
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
		t.Fatal("expected empty-file recovery to apply validated specialist output")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "function Clock") {
		t.Fatalf("target content = %q, want validated Clock implementation", content)
	}
	if len(code.inputs) != 1 || code.inputs[0].WorkItem.Path != "src/Clock.js" {
		t.Fatalf("specialist inputs = %#v, want exact queued target", code.inputs)
	}
	if !structuredEventsContain(events, "empty_file_recovery_applied") {
		t.Fatalf("missing empty-file apply event: %#v", events)
	}
}

func TestProgressionGateEmptyFileRecoveryRejectsInvalidSpecialistContentWithoutFallback(t *testing.T) {
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
		Reason:           "empty project files remain; empty-file recovery required",
		RecoveryToolTask: emptyProjectFilesRecoveryToolTask("Finish QA on this React app", nil, workspace),
	}
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{Content: "", Rationale: "invalid empty attempt one"},
		{Content: "", Rationale: "invalid empty attempt two"},
		{Content: "", Rationale: "invalid empty attempt three"},
	}}
	result := CommandDecisionResult{}
	events := []StructuredCommandEvent{}
	handled, err := runProgressionGateRecovery(
		context.Background(),
		4,
		"Finish QA on this React app",
		decision,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace, CodeContentSpecialist: code},
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(evt StructuredCommandEvent) { events = append(events, evt) },
		nil,
		&result,
	)
	if err == nil {
		t.Fatal("expected invalid specialist output to fail recovery")
	}
	if !handled {
		t.Fatal("empty-file recovery must own and report invalid specialist output")
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(content) != 0 {
		t.Fatalf("invalid specialist output triggered fallback content: %q", content)
	}
	if !structuredEventsContain(events, "empty_file_recovery_failed") {
		t.Fatalf("missing explicit empty-file failure event: %#v", events)
	}
	if structuredEventsContain(events, "empty_file_recovery_applied") {
		t.Fatalf("invalid specialist output emitted forbidden apply event: %#v", events)
	}
}

func TestArchitectFileWorkDoesNotFallThroughToShellPathSelection(t *testing.T) {
	workspace := t.TempDir()
	prompt := "Build a React notes app"
	toolTask := "Recovery required. Implementation architect target root: . Create or modify the actual project files."
	survey := WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM}
	contract := buildImplementationArchitectContract(prompt, toolTask, workspace, survey, nil)
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json", "vite.config.js", "index.html", "src/main.jsx", "scripts/smoke-test.mjs")
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
		prompt,
		toolTask,
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: workspace, ShellSpecialist: shell},
		survey,
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

func TestArchitectLaneReadsExistingFileBeforeUpdatingIt(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"scripts":{"test":"old","build":"old"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result := CommandDecisionResult{}
	handled, err := runArchitectCodeContentLane(
		context.Background(),
		2,
		"Build a React app",
		"Implementation architect target root: . Create or modify the actual project files.",
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: workspace,
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
	if !structuredEventsContain(events, "package_metadata_updated") || !structuredEventsContain(events, "dependency_metadata_configured") || !structuredEventsContain(events, "package_dependencies_declared") || !structuredEventsContain(events, "scripts_configured") || !structuredEventsContain(events, "package_json_valid") {
		t.Fatalf("missing package metadata evidence events: %#v", events)
	}
	if structuredEventsContain(events, "dependencies_installed") {
		t.Fatalf("package metadata handler must not claim npm install ran: %#v", events)
	}
	if len(result.Observations) == 0 || strings.Contains(result.Observations[len(result.Observations)-1].Stdout, "dependencies_installed") {
		t.Fatalf("package metadata observation used install evidence wording: %#v", result.Observations)
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
	installOfflineNPMArchitectStub(t)
	workspace := t.TempDir()
	var nilCodex *CodexSDKArchitectAgent
	code := &fakeCodeContentSpecialist{proposals: []CodeContentProposal{
		{
			Content:   "import React, { useState } from 'react';\n\nexport default function App() {\n  const [level, setLevel] = useState(1);\n  return React.createElement('main', { className: 'notes' },\n    React.createElement('h1', null, 'Notes'),\n    React.createElement('button', { type: 'button', onClick: () => setLevel(level + 1) }, 'Add note'),\n    React.createElement('input', { type: 'range', value: level, onChange: (event) => setLevel(Number(event.target.value)) })\n  );\n}\n",
			Rationale: "local source implementation",
		},
		{
			Content:   ".notes { display: grid; gap: 1rem; }\n.notes button { color: white; background: navy; }\n",
			Rationale: "local notes stylesheet",
		},
	}}
	result := CommandDecisionResult{Observations: []StructuredCommandObservation{
		{Command: "architect.apply create package.json", ExitCode: 0},
		{Command: "architect.apply create vite.config.js", ExitCode: 0},
		{Command: "architect.apply create index.html", ExitCode: 0},
		{Command: "architect.apply create src/main.jsx", ExitCode: 0},
		{Command: "architect.apply create scripts/smoke-test.mjs", ExitCode: 0},
	}}
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		workspace,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		result.Observations,
	)
	seedReactArchitectFileEvidence(t, workspace, contract,
		"package.json",
		"vite.config.js",
		"index.html",
		"src/main.jsx",
		"scripts/smoke-test.mjs",
	)
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
