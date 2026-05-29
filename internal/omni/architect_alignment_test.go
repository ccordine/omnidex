package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateArchitectContentAlignsWithPromptRejectsStudioInNoteApp(t *testing.T) {
	contract := ImplementationArchitectContract{
		AcceptanceCriteria: []string{"note list", "add note", "delete note"},
	}
	item := ArchitectWorkItem{Operation: "create", CWD: ".", Path: "src/App.js"}
	studioContent := `export default function App() {
  return (
    <main className="studio-shell">
      <section className="channel-rack">Channel Rack</section>
      <section className="piano-roll">Piano Roll</section>
    </main>
  );
}
`
	err := validateArchitectContentAlignsWithPrompt(studioContent, item, "Build a React note-taking app with CRUD notes", contract)
	if err == nil || (!strings.Contains(err.Error(), "notes app") && !strings.Contains(err.Error(), "music studio")) {
		t.Fatalf("expected note-app or music-studio alignment rejection, got %v", err)
	}
}

func TestValidateArchitectContentAlignsWithPromptRejectsStudioForGraphingCalculator(t *testing.T) {
	contract := ImplementationArchitectContract{
		AcceptanceCriteria: []string{"graphing calculator", "function plot", "equation input"},
	}
	item := ArchitectWorkItem{Operation: "create", CWD: ".", Path: "src/App.js"}
	studioContent := `export default function App() {
  return React.createElement('main', { className: 'studio-shell' }, 'Fruity Loops Inspired Music Production Studio');
}`
	err := validateArchitectContentAlignsWithPrompt(studioContent, item, "Build a React JS graphing calculator app", contract)
	if err == nil || (!strings.Contains(err.Error(), "graphing calculator") && !strings.Contains(err.Error(), "music studio")) {
		t.Fatalf("expected graphing calculator alignment rejection, got %v", err)
	}
}

func TestValidateArchitectContentAlignsWithPromptAcceptsNoteAppContent(t *testing.T) {
	contract := ImplementationArchitectContract{
		AcceptanceCriteria: []string{"note list", "add note", "delete note"},
	}
	item := ArchitectWorkItem{Operation: "create", CWD: ".", Path: "src/App.js"}
	noteContent := `export default function App() {
  return (
    <main className="app">
      <h1>Note Manager</h1>
      <section className="note-list">Notes</section>
      <button onClick={() => {}}>Add note</button>
      <button onClick={() => {}}>Delete note</button>
    </main>
  );
}
`
	if err := validateArchitectContentAlignsWithPrompt(noteContent, item, "Build a React note-taking app with CRUD notes", contract); err != nil {
		t.Fatalf("expected note app content to pass alignment: %v", err)
	}
}

func TestArchitectWorkItemFileEvidenceValidReadsDisk(t *testing.T) {
	workspace := t.TempDir()
	contract := buildImplementationArchitectContract(
		"Build a React note-taking app.",
		"Implementation architect target root: .",
		workspace,
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	item := ArchitectWorkItem{ID: "create_note_app", Operation: "create", CWD: ".", Path: "src/App.js"}
	content := deterministicGenericReactApp(contract)
	target := filepath.Join(workspace, "src", "App.js")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	contract.SourcePrompt = "Build a React note-taking app."
	if _, err := architectWorkItemFileEvidenceValid(item, workspace, contract, contract.SourcePrompt); err != nil {
		t.Fatalf("expected on-disk note app evidence to validate: %v", err)
	}
}

func TestArchitectImplementationBlockedByMissingTestProbe(t *testing.T) {
	workspace := t.TempDir()
	contract := buildImplementationArchitectContract(
		"Build a React note-taking app.",
		"Implementation architect target root: .",
		workspace,
		WorksiteSurvey{PackageManager: packageManagerNPM},
		nil,
	)
	appItem := ArchitectWorkItem{ID: "create_react_entrypoint", Operation: "create", CWD: ".", Path: "src/App.js"}
	err := architectImplementationBlockedByMissingTestProbe(contract.WorkQueue, appItem, workspace, contract, "Build a React note-taking app.", nil)
	if err == nil {
		t.Fatal("expected test-first gate to block App.js before smoke test is applied")
	}
	if !strings.Contains(err.Error(), "test_first gate") {
		t.Fatalf("unexpected gate error: %v", err)
	}
}

func TestMemoryBriefLooksForeignToPromptFiltersStudioMemoryForNoteApp(t *testing.T) {
	memory := SessionMemory{
		Content: "Previous project used studio-shell channel-rack piano-roll beat studio layout",
	}
	if !memoryBriefLooksForeignToPrompt(memory, "Build a React note-taking app", "") {
		t.Fatal("expected studio memory to look foreign to note app prompt")
	}
}

func TestSeededSmokeTestPassesFileEvidenceForNotesApp(t *testing.T) {
	workspace := t.TempDir()
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		workspace,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		[]StructuredCommandObservation{{Command: "architect.apply create scripts/smoke-test.mjs", ExitCode: 0}},
	)
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json", "scripts/smoke-test.mjs")
	item := ArchitectWorkItem{ID: "write_react_acceptance_test", Operation: "create", CWD: ".", Path: "scripts/smoke-test.mjs"}
	if _, err := architectWorkItemFileEvidenceValid(item, workspace, contract, "Build a React notes app"); err != nil {
		t.Fatalf("seeded smoke test should validate: %v", err)
	}
}

func TestArchitectWorkItemSatisfiedRequiresValidOnDiskEvidence(t *testing.T) {
	workspace := t.TempDir()
	obs := []StructuredCommandObservation{{Command: "architect.apply create package.json", ExitCode: 0}}
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: . Create or modify the actual project files.",
		workspace,
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		obs,
	)
	item := ArchitectWorkItem{ID: "setup_react_package_metadata", Operation: "create", CWD: ".", Path: "package.json"}
	if architectWorkItemSatisfied(item, workspace, contract, obs) {
		t.Fatal("apply observation alone must not satisfy package.json without on-disk evidence")
	}
	seedReactArchitectFileEvidence(t, workspace, contract, "package.json")
	if _, err := architectWorkItemFileEvidenceValid(item, workspace, contract, "Build a React notes app"); err != nil {
		t.Fatalf("package.json file evidence failed: %v", err)
	}
	if !architectWorkItemSatisfied(item, workspace, contract, obs) {
		t.Fatal("package.json with valid on-disk evidence should satisfy work item")
	}
}

func TestCSSMustIncludeForContractUsesNoteAppSignals(t *testing.T) {
	contract := ImplementationArchitectContract{
		AcceptanceCriteria: []string{"note list", "add note"},
	}
	includes := cssMustIncludeForContract(contract)
	found := false
	for _, signal := range includes {
		if strings.Contains(signal, "note") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected note-related CSS signals, got %#v", includes)
	}
}
