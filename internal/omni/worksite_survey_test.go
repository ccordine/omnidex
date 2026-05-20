package omni

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorksiteSurveyDetectsExistingReactApp(t *testing.T) {
	dir := createReactFixture(t)
	survey := BuildWorksiteSurvey(dir).WithOperation(userOperationModifyExisting)
	if survey.UserOperation != userOperationModifyExisting {
		t.Fatalf("operation=%q", survey.UserOperation)
	}
	if survey.ProjectState != projectStateExistingReactApp {
		t.Fatalf("project state=%q evidence=%v", survey.ProjectState, survey.Evidence)
	}
	if survey.PackageManager != packageManagerNPM {
		t.Fatalf("package manager=%q", survey.PackageManager)
	}
	if !stringListContains(survey.Frameworks, "react") {
		t.Fatalf("frameworks=%v", survey.Frameworks)
	}
}

func TestWorksiteSurveyDetectsEmptyDirectoryCreateMode(t *testing.T) {
	dir := t.TempDir()
	survey := BuildWorksiteSurvey(dir).WithOperation(userOperationCreateNewProject)
	if survey.ProjectState != projectStateEmptyDirectory {
		t.Fatalf("project state=%q evidence=%v", survey.ProjectState, survey.Evidence)
	}
	if !survey.AllowedOperation {
		t.Fatalf("create in empty directory should be allowed: %#v", survey)
	}
}

func TestRecipeSelectorRejectsCreateNewRecipeForModifyExisting(t *testing.T) {
	survey := WorksiteSurvey{
		UserOperation:    userOperationModifyExisting,
		ProjectState:     projectStateExistingReactApp,
		AllowedOperation: true,
	}
	recipes := []Recipe{
		{ID: "react.create_new", Description: "Create React", Operation: userOperationCreateNewProject, RequiredProjectStates: []string{projectStateEmptyDirectory}, Objectives: []RecipeObjective{{ID: "create_new_react_project", Description: "Create new React project"}}, AllowedCommands: []string{"npm"}, EvidenceRequired: []string{"package.json"}},
		{ID: "react.modify_existing", Description: "Modify React", Operation: userOperationModifyExisting, RequiredProjectStates: []string{projectStateExistingReactApp}, Objectives: []RecipeObjective{{ID: "implement_calculator_ui", Description: "Implement calculator UI"}}, AllowedCommands: []string{"npm"}, EvidenceRequired: []string{"src"}},
	}
	selected := FilterRecipesForWorksiteSurvey(recipes, survey)
	if len(selected) != 1 || selected[0].ID != "react.modify_existing" {
		t.Fatalf("selected=%#v", selected)
	}
}

func TestObjectiveLedgerFiltersCreateNewObjectiveForModifyExisting(t *testing.T) {
	survey := WorksiteSurvey{UserOperation: userOperationModifyExisting, ProjectState: projectStateExistingReactApp}
	filtered := filterObjectiveLedgerForWorksiteSurvey([]StructuredObjective{
		{ID: "create_new_react_project", Description: "Create a new React project", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
		{ID: "implement_calculator_logic", Description: "Implement calculator logic", Status: "pending", Source: structuredObjectiveSourceUserExplicit, Required: true},
	}, survey)
	if len(filtered) != 1 || filtered[0].ID != "implement_calculator_logic" {
		t.Fatalf("filtered=%#v", filtered)
	}
}

func createReactFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"latest","react-dom":"latest"},"devDependencies":{"vite":"latest"},"scripts":{"build":"vite build"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "App.jsx"), []byte(`export default function App(){return <main/>}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vite.config.js"), []byte(`export default {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
