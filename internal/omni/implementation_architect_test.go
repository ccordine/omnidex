package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildImplementationArchitectContractTargetsNestedReactRoot(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"react-scripts build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package-lock.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "public", "index.html"), []byte(`<div id="root"></div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "src", "index.js"), []byte(`import './App';`), 0o644); err != nil {
		t.Fatal(err)
	}

	contract := buildImplementationArchitectContract("build a React music app", "Recovery required. Implement the app.", workspace, WorksiteSurvey{}, nil)
	if contract.TargetRoot != "react-music-production" {
		t.Fatalf("target root = %q", contract.TargetRoot)
	}
	if contract.Framework != "react" {
		t.Fatalf("framework = %q", contract.Framework)
	}
	if len(contract.ProofCommands) != 1 || contract.ProofCommands[0] != "cd react-music-production && npm run build" {
		t.Fatalf("proof commands = %#v", contract.ProofCommands)
	}
	if !stringSliceContains(contract.EditSurface, "react-music-production/src/App.js") {
		t.Fatalf("edit surface = %#v", contract.EditSurface)
	}
	if !stringSliceContains(contract.ValidatorScopes, "alignment_validator: after implementation evidence exists, check the completed work against user objectives without adding unrequested expectations.") {
		t.Fatalf("validator scopes = %#v", contract.ValidatorScopes)
	}
	if contract.CurrentItem == nil || contract.CurrentItem.ID != "read_before_setup_react_package_metadata" || contract.CurrentItem.Operation != "read" || contract.CurrentItem.CWD != "react-music-production" || contract.CurrentItem.Path != "package.json" {
		t.Fatalf("current item = %#v", contract.CurrentItem)
	}
}

func TestBuildImplementationArchitectContractIncludesReactBuildPrerequisites(t *testing.T) {
	contract := buildImplementationArchitectContract(
		"Build a React JS browser-based music production studio with pattern step sequencer, channel rack, mixer controls, transport controls, tempo control, piano roll or note grid, sample/instrument pads, visual timeline, and a polished production-studio UI.",
		"Implementation architect target root: . Create or modify the actual project files.",
		t.TempDir(),
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	got := []string{}
	for _, item := range contract.WorkQueue {
		got = append(got, item.ID+":"+item.Path+":"+item.Verify)
	}
	for _, want := range []string{
		"setup_react_package_metadata:package.json:npm test",
		"create_react_html_shell:index.html:npm test",
		"create_react_mount_entry:src/main.jsx:npm test",
		"write_react_acceptance_test:scripts/smoke-test.mjs:npm test",
		"install_react_dependencies::npm install",
		"verify_react_build::npm run build",
	} {
		if !stringSliceContains(got, want) {
			t.Fatalf("work queue missing %q: %#v", want, got)
		}
	}
	for _, want := range []string{"pattern step sequencer", "channel rack", "mixer controls", "transport controls", "tempo control", "piano roll", "note grid", "sample/instrument pads", "visual timeline", "production-studio ui"} {
		if !stringSliceContains(contract.AcceptanceCriteria, want) {
			t.Fatalf("acceptance criteria missing %q: %#v", want, contract.AcceptanceCriteria)
		}
	}
}

func TestBuildImplementationArchitectContractTypesWritesFromFilesystemEvidence(t *testing.T) {
	root := t.TempDir()
	contract := buildImplementationArchitectContract(
		"Build a React app.",
		"Implementation architect target root: . Create or modify the actual project files.",
		root,
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	if got := architectWorkQueueItemOperation(contract.WorkQueue, "setup_react_package_metadata"); got != "create" {
		t.Fatalf("empty project package operation = %q, want create", got)
	}
	if architectWorkQueueContainsID(contract.WorkQueue, "read_before_setup_react_package_metadata") {
		t.Fatalf("empty project should not read package.json before create: %#v", contract.WorkQueue)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"test":"node scripts/smoke-test.mjs","build":"vite build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	contract = buildImplementationArchitectContract(
		"Build a React app.",
		"Implementation architect target root: . Create or modify the actual project files.",
		root,
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	if got := architectWorkQueueItemOperation(contract.WorkQueue, "setup_react_package_metadata"); got != "update" {
		t.Fatalf("existing package operation = %q, want update", got)
	}
	if !architectReadImmediatelyPrecedesItem(contract.WorkQueue, "read_before_setup_react_package_metadata", "setup_react_package_metadata") {
		t.Fatalf("existing package update should have read immediately before update: %#v", contract.WorkQueue)
	}
	if contract.CurrentItem == nil || contract.CurrentItem.ID != "read_before_setup_react_package_metadata" || contract.CurrentItem.Operation != "read" {
		t.Fatalf("current item should be read before update, got %#v", contract.CurrentItem)
	}
}

func TestArchitectWriteOperationStaysStableAfterObservedCreate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"generated"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	contract := buildImplementationArchitectContract(
		"Build a React app.",
		"Implementation architect target root: . Create or modify the actual project files.",
		root,
		WorksiteSurvey{PackageManager: packageManagerNPM},
		[]StructuredCommandObservation{{Command: "architect.apply create package.json", ExitCode: 0}},
	)
	if got := architectWorkQueueItemOperation(contract.WorkQueue, "setup_react_package_metadata"); got != "create" {
		t.Fatalf("observed create should keep operation stable, got %q", got)
	}
	if architectWorkQueueContainsID(contract.WorkQueue, "read_before_setup_react_package_metadata") {
		t.Fatalf("observed create should not become update/read-before work: %#v", contract.WorkQueue)
	}
}

func TestArchitectApplyObservationMatchesCreateUpdateWriteEvidence(t *testing.T) {
	item := ArchitectWorkItem{Operation: "update", CWD: ".", Path: "package.json"}
	obs := StructuredCommandObservation{Command: "architect.apply create package.json", ExitCode: 0}
	if !architectApplyObservationMatches(item, obs) {
		t.Fatal("create evidence should satisfy update-class write item for same path")
	}
}

func architectWorkQueueItemOperation(queue []ArchitectWorkItem, id string) string {
	for _, item := range queue {
		if item.ID == id {
			return item.Operation
		}
	}
	return ""
}

func architectWorkQueueContainsID(queue []ArchitectWorkItem, id string) bool {
	for _, item := range queue {
		if item.ID == id {
			return true
		}
	}
	return false
}

func architectReadImmediatelyPrecedesItem(queue []ArchitectWorkItem, readID, itemID string) bool {
	for i := 1; i < len(queue); i++ {
		if queue[i-1].ID == readID && queue[i].ID == itemID {
			return true
		}
	}
	return false
}

func TestValidateReactStylesheetRejectsUnmatchedPlaceholderCSS(t *testing.T) {
	contract := buildImplementationArchitectContract(
		"Build a React JS browser-based music production studio with channel rack, mixer controls, visual timeline, piano roll, and sample/instrument pads.",
		"Implementation architect target root: . Create or modify the actual project files.",
		t.TempDir(),
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	item := ArchitectWorkItem{ID: "style_react_app", Operation: "update", CWD: ".", Path: "src/App.css"}
	content := `body { color: white; }
.studio-surface { padding: 20px; }
.channel-rack, .mixer-controls, .transport-controls, .piano-roll, .sample-pads, .visual-timeline { display: flex; }
/* Add more styles as needed */
`
	if err := validateCodeContentProposalForArchitectItem(content, contract, item); err == nil {
		t.Fatal("expected unmatched placeholder CSS to be rejected")
	}
	if err := validateCodeContentProposalForArchitectItem(deterministicReactMusicStudioCSS(), contract, item); err != nil {
		t.Fatalf("deterministic CSS should pass tightened selector validation: %v", err)
	}
}

func TestBuildImplementationArchitectContractRepairsMissingViteEntry(t *testing.T) {
	observations := []StructuredCommandObservation{
		{
			Command:  "cd react-music-production && npm run build",
			ExitCode: 1,
			Stderr:   "[vite]: Rollup failed to resolve import \"/src/main.js\" from \"/tmp/index.html\".",
		},
		{
			Command:  "cd react-music-production && sed -n '1,120p' index.html",
			ExitCode: 0,
			Stdout:   "<script type=\"module\" src=\"/src/main.js\"></script>",
		},
	}
	contract := buildImplementationArchitectContract("build a React music app", "Implementation architect target root: react-music-production. Continue implementation.", t.TempDir(), WorksiteSurvey{PackageManager: packageManagerNPM}, observations)
	if contract.CurrentItem == nil {
		t.Fatal("expected missing Vite entry repair item")
	}
	if contract.CurrentItem.ID != "repair_missing_vite_entry_src_main_js" {
		t.Fatalf("repair id = %q", contract.CurrentItem.ID)
	}
	if contract.CurrentItem.Operation != "update" || contract.CurrentItem.CWD != "react-music-production" || contract.CurrentItem.Path != "src/main.js" || contract.CurrentItem.Verify != "npm run build" {
		t.Fatalf("repair item = %#v", contract.CurrentItem)
	}
}

func TestBuildImplementationArchitectContractRepairsViteSyntaxErrorFile(t *testing.T) {
	observations := []StructuredCommandObservation{{
		Command:  "npm run build",
		ExitCode: 1,
		Stderr:   "[builtin:vite-transform] Unexpected token\n  ╭─[ src/App.js:40:5 ]\n",
	}}
	contract := buildImplementationArchitectContract("build a React music app", "Implementation architect target root: . Continue implementation.", t.TempDir(), WorksiteSurvey{PackageManager: packageManagerNPM}, observations)
	if contract.CurrentItem == nil {
		t.Fatal("expected syntax error repair item")
	}
	if contract.CurrentItem.Path != "src/App.js" || contract.CurrentItem.Operation != "update" || contract.CurrentItem.Verify != "npm run build" {
		t.Fatalf("repair item = %#v", contract.CurrentItem)
	}
}

func TestBuildImplementationArchitectContractRepairsCSSPostFailure(t *testing.T) {
	observations := []StructuredCommandObservation{{
		Command:  "npm run build",
		ExitCode: 1,
		Stderr:   "[plugin vite:css-post]\nSyntaxError: [lightningcss minify] Invalid dangling combinator in selector\n1  |  /* App.js */\n2  |  import React from 'react';",
	}}
	contract := buildImplementationArchitectContract("build a React music app", "Implementation architect target root: . Continue implementation.", t.TempDir(), WorksiteSurvey{PackageManager: packageManagerNPM}, observations)
	if contract.CurrentItem == nil {
		t.Fatal("expected css repair item")
	}
	if contract.CurrentItem.Path != "src/App.css" || contract.CurrentItem.Operation != "update" || contract.CurrentItem.Verify != "npm run build" {
		t.Fatalf("repair item = %#v", contract.CurrentItem)
	}
}

func TestBuildImplementationArchitectContractClearsMissingViteEntryAfterRepairOrPassingBuild(t *testing.T) {
	failedBuild := StructuredCommandObservation{
		Command:  "cd react-music-production && npm run build",
		ExitCode: 1,
		Stderr:   "Failed to resolve /src/main.js from /tmp/index.html",
	}
	afterApply := []StructuredCommandObservation{
		failedBuild,
		{Command: "architect.apply update react-music-production/src/main.js", ExitCode: 0, Stdout: "wrote src/main.js"},
	}
	contract := buildImplementationArchitectContract("build a React music app", "Implementation architect target root: react-music-production. Continue implementation.", t.TempDir(), WorksiteSurvey{PackageManager: packageManagerNPM}, afterApply)
	if contract.CurrentItem != nil && contract.CurrentItem.ID == "repair_missing_vite_entry_src_main_js" {
		t.Fatalf("repair should clear after matching apply: %#v", contract.CurrentItem)
	}

	afterPassingBuild := []StructuredCommandObservation{
		failedBuild,
		{Command: "cd react-music-production && npm run build", ExitCode: 0, Stdout: "built"},
	}
	contract = buildImplementationArchitectContract("build a React music app", "Implementation architect target root: react-music-production. Continue implementation.", t.TempDir(), WorksiteSurvey{PackageManager: packageManagerNPM}, afterPassingBuild)
	if contract.CurrentItem != nil && contract.CurrentItem.ID == "repair_missing_vite_entry_src_main_js" {
		t.Fatalf("repair should clear after passing build: %#v", contract.CurrentItem)
	}
}

func TestShellSpecialistRequestIncludesArchitectContract(t *testing.T) {
	req := buildShellCommandSpecialistRequest(ShellCommandSpecialistInput{
		UserPrompt: "build a React app",
		ToolTask:   "Implementation architect target root: react-music-production. Create source files.",
		ArchitectContract: ImplementationArchitectContract{
			Role:          "implementation_architect",
			TargetRoot:    "react-music-production",
			Framework:     "react",
			EditSurface:   []string{"react-music-production/src/App.js"},
			ProofCommands: []string{"cd react-music-production && npm run build"},
		},
	})
	text := structuredRequestMessagesText(req)
	for _, want := range []string{
		"architect_contract",
		"current_item",
		"implementation_architect",
		"react-music-production",
		"the implementation architect's authority",
		"satisfy only that one queued operation",
		"coder/shell specialist only chooses the next concrete command",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("request missing %q: %s", want, text)
		}
	}
}

func TestStructuredEvaluationRequestCarriesValidationScope(t *testing.T) {
	req := buildStructuredLLMEvaluationRequest(StructuredLLMEvaluationInput{
		Step:            2,
		UserPrompt:      "build a React app",
		PlannerJob:      "choose next command",
		ValidationScope: "current_objective_and_payload_shape",
		LLMResponse:     `{"command":"npm run build","done":false}`,
	})
	text := structuredRequestMessagesText(req)
	for _, want := range []string{
		`"validation_scope":"current_objective_and_payload_shape"`,
		"Validation scope is authoritative",
		"Do not act as a broad product critic",
		"alignment_after_evidence",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("evaluation request missing %q: %s", want, text)
		}
	}
}

func TestValidateCommandAgainstImplementationArchitectContractRequiresTargetRoot(t *testing.T) {
	contract := ImplementationArchitectContract{TargetRoot: "react-music-production"}
	if err := validateCommandAgainstImplementationArchitectContract("cat > src/App.js <<'JS'\nexport default function App() { return null; }\nJS", contract); err == nil {
		t.Fatal("expected root-relative edit to be rejected")
	}
	if err := validateCommandAgainstImplementationArchitectContract("cd react-music-production && npm run build", contract); err != nil {
		t.Fatalf("expected target-root command to be accepted: %v", err)
	}
}

func TestValidateCommandAgainstImplementationArchitectCurrentItem(t *testing.T) {
	contract := ImplementationArchitectContract{
		TargetRoot:  "react-music-production",
		CurrentItem: &ArchitectWorkItem{ID: "create_react_entrypoint", Operation: "update", CWD: "react-music-production", Path: "src/App.js"},
	}
	if err := validateCommandAgainstImplementationArchitectContract("cd react-music-production && sed -n '1,160p' src/App.css", contract); err != nil {
		t.Fatalf("read-only inspection should remain allowed during architect work: %v", err)
	}
	if err := validateCommandAgainstImplementationArchitectContract("cd react-music-production && cat > src/App.css <<'CSS'\nbody{}\nCSS", contract); err == nil {
		t.Fatal("expected wrong current item path to be rejected")
	}
	if err := validateCommandAgainstImplementationArchitectContract("cd react-music-production && cat > src/App.js <<'JS'\nexport default function App() { return null; }\nJS", contract); err != nil {
		t.Fatalf("expected current item path to be accepted: %v", err)
	}
}

func TestBuildImplementationArchitectContractSkipsNonImplementationTask(t *testing.T) {
	contract := buildImplementationArchitectContract("what is the weather", "Use wttr.in for current weather.", t.TempDir(), WorksiteSurvey{}, nil)
	if hasImplementationArchitectContract(contract) {
		t.Fatalf("unexpected contract: %#v", contract)
	}
}

func TestValidateStructuredCommandForRunWithArchitectRejectsDirectWrongEdit(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"react-scripts build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	toolTask := "Implementation architect target root: react-music-production. Create or modify the actual project files."
	err := validateStructuredCommandForRunWithArchitect("cd react-music-production && cat > src/App.css <<'CSS'\nbody{}\nCSS", "build a React music production app", toolTask, "", nil, workspace, []StructuredObjective{{ID: "implement_ui", Description: "implement UI", Status: "pending", Required: true}}, WorksiteSurvey{})
	if err == nil {
		t.Fatal("expected direct edit outside current test-first item to be rejected")
	}
	if !strings.Contains(err.Error(), "setup_react_package_metadata") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateStructuredCommandForRunWithArchitectRejectsDirectBuildDuringRepair(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Command: "architect.apply update package.json", ExitCode: 0},
		{Command: "cd . && npm run build", ExitCode: 1, Stderr: "Error: Failed to resolve /src/main.jsx from /tmp/index.html"},
	}
	err := validateStructuredCommandForRunWithArchitect("npm run build", "build a React music production app", "", "", observations, t.TempDir(), []StructuredObjective{{ID: "build_app", Status: "pending", Required: true}}, WorksiteSurvey{PackageManager: packageManagerNPM})
	if err == nil {
		t.Fatal("expected direct build retry to be rejected while missing-entry repair is current")
	}
	if !strings.Contains(err.Error(), "repair_missing_vite_entry_src_main_jsx") || !strings.Contains(err.Error(), "src/main.jsx") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShouldBypassEvaluatorForArchitectImplementationButNotReads(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "react-music-production")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "package.json"), []byte(`{"scripts":{"build":"react-scripts build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writePayload := `{"command":"cd react-music-production && printf '%s\\n' \"test('music app',()=>{});\" > src/App.test.js","done":false,"answer":"","tool_task":"Implementation architect target root: react-music-production. Create or modify the actual project files."}`
	if !shouldBypassEvaluatorForArchitectImplementation(writePayload, "build a React music production app", workspace, WorksiteSurvey{}, nil) {
		t.Fatal("expected evaluator bypass for architect-owned write")
	}
	readPayload := `{"command":"pwd && find . -maxdepth 2 -type f","done":false,"answer":"","tool_task":"Inspect the workspace before implementation."}`
	if shouldBypassEvaluatorForArchitectImplementation(readPayload, "build a React music production app", workspace, WorksiteSurvey{}, nil) {
		t.Fatal("read-only inspection should not enter architect evaluator bypass")
	}
}
