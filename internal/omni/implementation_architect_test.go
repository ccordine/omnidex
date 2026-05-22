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
		"implementation_architect",
		"react-music-production",
		"the implementation architect's authority",
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
