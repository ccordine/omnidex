package omni

import (
	"strings"
	"testing"
)

func TestBuildProjectFileMapFromContractIncludesDependencies(t *testing.T) {
	contract := buildImplementationArchitectContract(
		"Build a React notes app",
		"Implementation architect target root: .",
		t.TempDir(),
		WorksiteSurvey{Frameworks: []string{"react"}, PackageManager: packageManagerNPM},
		nil,
	)
	projectMap := buildProjectFileMapFromContract(contract)
	if len(projectMap.Files) == 0 {
		t.Fatal("expected mapped files from architect contract")
	}
	app := projectFileMapEntryByPath(projectMap, "src/App.js")
	if app == nil {
		t.Fatal("expected src/App.js in project map")
	}
	if len(app.DependsOn) == 0 || !strings.Contains(strings.Join(app.DependsOn, ","), "smoke-test") {
		t.Fatalf("App.js should depend on smoke test, got %#v", app.DependsOn)
	}
	if projectMap.TreeSummary == "" {
		t.Fatal("expected tree summary")
	}
}

func TestProjectFileMapCurrentActiveRespectsDependencies(t *testing.T) {
	projectMap := ProjectFileMap{
		Files: []ProjectFileMapEntry{
			{Path: "package.json", Status: ProjectFileMapStatusPlanned, WorkItemID: "pkg"},
			{Path: "src/App.js", Status: ProjectFileMapStatusPlanned, DependsOn: []string{"package.json"}, WorkItemID: "app"},
		},
	}
	active := projectFileMapCurrentActive(projectMap)
	if active == nil || active.Path != "package.json" {
		t.Fatalf("expected package.json first, got %#v", active)
	}
	projectMap.Files[0].Status = ProjectFileMapStatusDone
	active = projectFileMapCurrentActive(projectMap)
	if active == nil || active.Path != "src/App.js" {
		t.Fatalf("expected src/App.js after package.json done, got %#v", active)
	}
}

func TestValidateCommandAgainstProjectFileMapRejectsUnmappedTarget(t *testing.T) {
	projectMap := ProjectFileMap{
		ActiveFile: &ProjectFileMapEntry{Path: "src/App.js"},
		Files: []ProjectFileMapEntry{
			{Path: "src/App.js", Status: ProjectFileMapStatusInProgress},
		},
	}
	err := validateCommandAgainstProjectFileMap("touch src/index.js", t.TempDir(), projectMap)
	if err == nil || !strings.Contains(err.Error(), "not in the active project file map") {
		t.Fatalf("expected unmapped target rejection, got %v", err)
	}
}

func TestValidateCommandAgainstProjectFileMapRejectsWrongActiveFile(t *testing.T) {
	projectMap := ProjectFileMap{
		ActiveFile: &ProjectFileMapEntry{Path: "index.html"},
		Files: []ProjectFileMapEntry{
			{Path: "index.html", Status: ProjectFileMapStatusInProgress},
			{Path: "src/App.js", Status: ProjectFileMapStatusPlanned},
		},
	}
	err := validateCommandAgainstProjectFileMap(`printf 'export default function App(){ return null; }' > src/App.js`, t.TempDir(), projectMap)
	if err == nil || !strings.Contains(err.Error(), "active mapped file is") {
		t.Fatalf("expected active file enforcement, got %v", err)
	}
}

func TestMarkProjectFileMapEntryDoneUpdatesOpenChanges(t *testing.T) {
	projectMap := ProjectFileMap{
		Revision: 1,
		Files: []ProjectFileMapEntry{
			{Path: "package.json", Status: ProjectFileMapStatusDone},
			{Path: "index.html", Status: ProjectFileMapStatusPlanned},
		},
	}
	projectMap = markProjectFileMapEntryDone(projectMap, "index.html")
	if projectMap.Revision != 2 {
		t.Fatalf("revision = %d, want 2", projectMap.Revision)
	}
	if len(projectMap.OpenChanges) != 0 {
		t.Fatalf("open changes = %#v", projectMap.OpenChanges)
	}
}
