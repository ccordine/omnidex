package worker

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationTaskStageProjectionExcludesOtherTasksAndApplicationEntrypoints(t *testing.T) {
	t.Parallel()

	input, frozen, program := applicationTaskLifecycleFixture(t)
	program.Generated = map[string]string{
		"feature.001":    "function Feature001View(): number { return 1; }",
		"acceptance.001": "function VerifyFeature001(): number { return 1; }",
		"feature.002":    "function Feature002View(): number { return 2; }",
		"acceptance.002": "function VerifyFeature002(): number { return 2; }",
	}
	context, err := assemblyline.ProjectApplicationTaskContext(input, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, context)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := taskStageDocumentIDs(stage), []string{"acceptance_001", "application_runtime", "feature_001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task stage documents=%v want=%v", got, want)
	}
	if got, want := taskStageStaticPaths(stage), []string{"package.json", "tsconfig.json", "vite.config.ts"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task stage static paths=%v want=%v", got, want)
	}
	if got, want := sortedGeneratedIDs(stage.Generated), []string{"acceptance.001", "feature.001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task stage generated=%v want=%v", got, want)
	}
	for _, forbidden := range []string{
		"application_shell", "application_smoke_test", "application_runtime_test",
		"feature_002", "acceptance_002", "src/main.tsx", "src/App.tsx", "index.html",
	} {
		if strings.Contains(renderTaskStageIdentity(stage), forbidden) {
			t.Fatalf("task stage exposed unrelated identity %q", forbidden)
		}
	}
	if _, err := directCodingTypeScriptCorrectionBlock(stage.TypeScript, "feature.002"); err == nil {
		t.Fatal("task stage allowed correction of another task block")
	}
	routed, err := routeDirectCodingAcceptanceFailure(
		stage.TypeScript,
		&directCodingStageDiagnostic{
			BlockID: "acceptance.001", Message: "current acceptance failed",
			FailureClass: directCodingStageFailureVitestBehavior,
		},
	)
	if err != nil || routed.BlockID != "feature.001" {
		t.Fatalf("current acceptance routing=%+v error=%v", routed, err)
	}
}

func applicationTaskLifecycleFixture(
	t *testing.T,
) (assemblyline.ApplicationWorkloadDraftInput, assemblyline.FrozenApplicationWorkload, directCodingProgram) {
	t.Helper()
	input := assemblyline.ApplicationWorkloadDraftInput{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "small operations console",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "group records"},
			{ID: "requirement_002", SourceQuote: "filter records"},
		},
	}
	draft := assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{
			{
				RequirementID: "requirement_001",
				Objective:     "Implement interactive record grouping in the operations console.",
				RequiredBehaviors: []string{
					"Users can create a named record group.",
					"Users can assign a visible record to that group.",
				},
				AcceptanceCriteria: []string{
					"A created group is visible.",
					"An assigned record is visible in its selected group.",
				},
			},
			{
				RequirementID: "requirement_002",
				Objective:     "Implement interactive record filtering in the operations console.",
				RequiredBehaviors: []string{
					"Users can enter a record filter.",
					"Users can clear the active record filter.",
				},
				AcceptanceCriteria: []string{
					"Entering a filter changes the visible records to matching results.",
					"Clearing the filter restores all visible records.",
				},
			},
		},
	}
	frozen, err := assemblyline.FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{
		Adapter: "test", PackageName: "test", Workload: frozen, Generated: map[string]string{},
		TypeScript: applicationTaskLifecycleBlueprint(),
		StaticFiles: []directCodingFileTask{
			{Path: "package.json", Content: `{}`}, {Path: "tsconfig.json", Content: `{}`},
			{Path: "vite.config.ts", Content: "export default {};"},
			{Path: "index.html", Content: `<div id="root"></div>`},
			{Path: "src/main.tsx", Content: "import { App } from './App';"},
			{Path: "src/styles.css", Content: "body {}"},
		},
	}
	if err := program.TypeScript.Validate(); err != nil {
		t.Fatal(err)
	}
	return input, frozen, program
}

func applicationTaskLifecycleBlueprint() assemblyline.TypeScriptBlueprint {
	documents := []assemblyline.TypeScriptDocument{{
		ID: "application_runtime", Path: "src/runtime.tsx", Blocks: []assemblyline.TypeScriptBlock{
			{ID: "runtime.api", Static: "interface Runtime {}", API: "interface Runtime {}"},
			{ID: "runtime.factory", Static: "function runtime(): number { return 1; }", API: "function runtime(): number", DependsOn: []string{"runtime.api"}},
		},
	}}
	for sequence := 1; sequence <= 2; sequence++ {
		suffix := formatTaskSequence(sequence)
		featureID := "feature." + suffix
		documents = append(documents,
			assemblyline.TypeScriptDocument{
				ID: "feature_" + suffix, Path: "src/features/Feature" + suffix + ".tsx",
				Blocks: []assemblyline.TypeScriptBlock{
					{ID: "feature.context." + suffix, Static: "interface Context" + suffix + " {}", API: "interface Context" + suffix + " {}", DependsOn: []string{"runtime.api"}},
					{ID: featureID, Signature: "function Feature" + suffix + "View(): number", Contract: "Return a value.", API: "function Feature" + suffix + "View(): number", DependsOn: []string{"feature.context." + suffix}},
					{ID: "feature.wrapper." + suffix, Static: "function Feature" + suffix + "(): number { return 1; }", API: "function Feature" + suffix + "(): number", DependsOn: []string{featureID}},
				},
			},
			assemblyline.TypeScriptDocument{
				ID: "acceptance_" + suffix, Path: "src/features/Feature" + suffix + ".test.tsx",
				Blocks: []assemblyline.TypeScriptBlock{
					{ID: "acceptance." + suffix, Signature: "function VerifyFeature" + suffix + "(): number", Contract: "Verify the feature.", API: "function VerifyFeature" + suffix + "(): number", DependsOn: []string{"runtime.factory", featureID}, FailureTarget: featureID},
					{ID: "acceptance.register." + suffix, Static: "void 0;", API: "registered acceptance " + suffix, DependsOn: []string{"acceptance." + suffix}},
				},
			},
		)
	}
	documents = append(documents,
		assemblyline.TypeScriptDocument{ID: "application_shell", Path: "src/App.tsx", Blocks: []assemblyline.TypeScriptBlock{{ID: "application.render", Static: "function App(): number { return 1; }", API: "function App(): number", DependsOn: []string{"feature.001", "feature.002"}}}},
		assemblyline.TypeScriptDocument{ID: "application_smoke_test", Path: "src/App.test.tsx", Blocks: []assemblyline.TypeScriptBlock{{ID: "tests.application_smoke", Static: "void 0;", API: "application smoke", DependsOn: []string{"application.render"}}}},
		assemblyline.TypeScriptDocument{ID: "application_runtime_test", Path: "src/runtime.test.ts", Blocks: []assemblyline.TypeScriptBlock{{ID: "tests.runtime", Static: "void 0;", API: "runtime test", DependsOn: []string{"runtime.factory"}}}},
	)
	return assemblyline.TypeScriptBlueprint{Documents: documents}
}

func formatTaskSequence(sequence int) string {
	return fmt.Sprintf("%03d", sequence)
}

func assertTaskStageOwnsOnly(t *testing.T, stage directCodingProgram, taskID string) {
	t.Helper()
	wantSuffix := strings.TrimPrefix(taskID, "task_")
	for _, id := range taskStageDocumentIDs(stage) {
		if strings.HasPrefix(id, "feature_") || strings.HasPrefix(id, "acceptance_") {
			if !strings.HasSuffix(id, wantSuffix) {
				t.Fatalf("stage for %s contains %s", taskID, id)
			}
		}
	}
}

func taskStageDocumentIDs(program directCodingProgram) []string {
	ids := make([]string, 0, len(program.TypeScript.Documents))
	for _, document := range program.TypeScript.Documents {
		ids = append(ids, document.ID)
	}
	sort.Strings(ids)
	return ids
}

func taskStageStaticPaths(program directCodingProgram) []string {
	paths := make([]string, 0, len(program.StaticFiles))
	for _, file := range program.StaticFiles {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return paths
}

func sortedGeneratedIDs(generated map[string]string) []string {
	ids := make([]string, 0, len(generated))
	for id := range generated {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func renderTaskStageIdentity(program directCodingProgram) string {
	return strings.Join(taskStageDocumentIDs(program), "\n") + "\n" + strings.Join(taskStageStaticPaths(program), "\n")
}
