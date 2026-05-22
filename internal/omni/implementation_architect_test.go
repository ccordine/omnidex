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
	if contract.CurrentItem == nil || contract.CurrentItem.ID != "write_react_acceptance_test" || contract.CurrentItem.CWD != "react-music-production" || contract.CurrentItem.Path != "src/App.test.js" {
		t.Fatalf("current item = %#v", contract.CurrentItem)
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

func TestValidateCommandAgainstImplementationArchitectContractRejectsPlaceholderPath(t *testing.T) {
	contract := ImplementationArchitectContract{TargetRoot: "react-music-production"}
	err := validateCommandAgainstImplementationArchitectContract("cd /path/to/your/project && npm test", contract)
	if err == nil {
		t.Fatal("expected placeholder path to be rejected")
	}
	if !strings.Contains(err.Error(), "placeholder project path") {
		t.Fatalf("unexpected error: %v", err)
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
	if !strings.Contains(err.Error(), "write_react_acceptance_test") {
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
