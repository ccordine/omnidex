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

	_, frozen, program := applicationTaskLifecycleFixture(t)
	program.Generated = map[string]string{
		"feature.001":    "function Feature001View(): number { return 1; }",
		"acceptance.001": "function VerifyFeature001(): number { return 1; }",
		"feature.002":    "function Feature002View(): number { return 2; }",
		"acceptance.002": "function VerifyFeature002(): number { return 2; }",
	}
	context, err := assemblyline.ProjectApplicationTaskContext(frozen, "task_001")
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
	if got, want := taskStageStaticPaths(stage), []string{"package-lock.json", "package.json", "tsconfig.json", "vite.config.ts"}; !reflect.DeepEqual(got, want) {
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
	if _, err := directCodingTypeScriptCorrectionBlock(stage.Source, "feature.002"); err == nil {
		t.Fatal("task stage allowed correction of another task block")
	}
	_, err = routeDirectCodingAcceptanceFailure(
		stage,
		&directCodingStageDiagnostic{
			BlockID: "acceptance.001", Message: "current acceptance failed",
			FailureClass: directCodingStageFailureVitestBehavior,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "cannot authorize implementation mutation") {
		t.Fatalf("current generated verification routing error=%v", err)
	}
}

func TestApplicationTaskStageProjectionSupportsMultipleTasksInOneFile(t *testing.T) {
	t.Parallel()

	_, frozen, program := applicationTaskLifecycleFixture(t)
	program.Source.Documents = coalesceApplicationTaskFixtureDocuments(t, program.Source.Documents)
	context, err := assemblyline.ProjectApplicationTaskContext(frozen, "task_002")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, context)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := directCodingSourceBlueprintBlock(stage.Source, "feature.002"); !exists {
		t.Fatal("grouped stage omits feature.002")
	}
	if _, exists := directCodingSourceBlueprintBlock(stage.Source, "acceptance.002"); !exists {
		t.Fatal("grouped stage omits acceptance.002")
	}
}

func coalesceApplicationTaskFixtureDocuments(
	t *testing.T,
	documents []assemblyline.SourceDocument,
) []assemblyline.SourceDocument {
	t.Helper()
	output := make([]assemblyline.SourceDocument, 0, len(documents)-2)
	features := assemblyline.SourceDocument{ID: "features", Path: "src/Counter.tsx", AdapterID: "typescript_react"}
	acceptance := assemblyline.SourceDocument{ID: "acceptance", Path: "src/Counter.test.tsx", AdapterID: "typescript_react"}
	for _, document := range documents {
		switch document.ID {
		case "feature_001", "feature_002":
			features.Blocks = append(features.Blocks, document.Blocks...)
		case "acceptance_001", "acceptance_002":
			acceptance.Blocks = append(acceptance.Blocks, document.Blocks...)
		default:
			output = append(output, document)
		}
	}
	output = append(output, features, acceptance)
	if err := (assemblyline.SourceBlueprint{Documents: output}).Validate(); err != nil {
		t.Fatal(err)
	}
	return output
}

func applicationTaskLifecycleFixture(
	t *testing.T,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload, directCodingProgram) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "small operations console",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "group records"},
			{ID: "requirement_002", SourceQuote: "filter records"},
		},
	}
	frozen, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, Workload: frozen, Generated: map[string]string{},
		Source: applicationTaskLifecycleBlueprint(),
		StaticFiles: []directCodingFileTask{
			{Path: "package.json", Content: `{}`}, {Path: "package-lock.json", Content: `{}`},
			{Path: "tsconfig.json", Content: `{}`},
			{Path: "vite.config.ts", Content: "export default {};"},
			{Path: "index.html", Content: `<div id="root"></div>`},
			{Path: "src/main.tsx", Content: "import { App } from './App';"},
			{Path: "src/styles.css", Content: "body {}"},
		},
	}
	if err := program.Source.Validate(); err != nil {
		t.Fatal(err)
	}
	return specification, frozen, program
}

func applicationTaskLifecycleBlueprint() assemblyline.SourceBlueprint {
	documents := []assemblyline.SourceDocument{{
		ID: "application_runtime", Path: "src/runtime.tsx", AdapterID: "typescript_react", Blocks: []assemblyline.SourceBlock{
			{ID: "runtime.api", Static: "interface Runtime {}", API: "interface Runtime {}"},
			{ID: "runtime.factory", Static: "function runtime(): number { return 1; }", API: "function runtime(): number", DependsOn: []string{"runtime.api"}},
		},
	}}
	for sequence := 1; sequence <= 2; sequence++ {
		suffix := formatTaskSequence(sequence)
		taskID := "task_" + suffix
		featureID := "feature." + suffix
		documents = append(documents,
			assemblyline.SourceDocument{
				ID: "feature_" + suffix, Path: "src/features/Feature" + suffix + ".tsx", AdapterID: "typescript_react",
				Blocks: []assemblyline.SourceBlock{
					{ID: "feature.context." + suffix, Static: "interface Context" + suffix + " {}", API: "interface Context" + suffix + " {}", DependsOn: []string{"runtime.api"}, TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport},
					{ID: featureID, Signature: "function Feature" + suffix + "View(): number", Contract: "Return a value.", API: "function Feature" + suffix + "View(): number", DependsOn: []string{"feature.context." + suffix}, TaskID: taskID, Role: assemblyline.SourceBlockTaskImplementation},
					{ID: "feature.wrapper." + suffix, Static: "function Feature" + suffix + "(): number { return 1; }", API: "function Feature" + suffix + "(): number", DependsOn: []string{featureID}, TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport},
				},
			},
			assemblyline.SourceDocument{
				ID: "acceptance_" + suffix, Path: "src/features/Feature" + suffix + ".test.tsx", AdapterID: "typescript_react",
				Blocks: []assemblyline.SourceBlock{
					{ID: "acceptance." + suffix, Signature: "function VerifyFeature" + suffix + "(): number", Contract: "Verify the feature.", API: "function VerifyFeature" + suffix + "(): number", DependsOn: []string{"runtime.factory", featureID}, TaskID: taskID, Role: assemblyline.SourceBlockTaskVerification},
					{ID: "acceptance.register." + suffix, Static: "void 0;", API: "registered acceptance " + suffix, DependsOn: []string{"acceptance." + suffix}, TaskID: taskID, Role: assemblyline.SourceBlockTaskSupport},
				},
			},
		)
	}
	documents = append(documents,
		assemblyline.SourceDocument{ID: "application_shell", Path: "src/App.tsx", AdapterID: "typescript_react", Blocks: []assemblyline.SourceBlock{{ID: "application.render", Static: "function App(): number { return 1; }", API: "function App(): number", DependsOn: []string{"feature.001", "feature.002"}}}},
		assemblyline.SourceDocument{ID: "application_smoke_test", Path: "src/App.test.tsx", AdapterID: "typescript_react", Blocks: []assemblyline.SourceBlock{{ID: "tests.application_smoke", Static: "void 0;", API: "application smoke", DependsOn: []string{"application.render"}}}},
		assemblyline.SourceDocument{ID: "application_runtime_test", Path: "src/runtime.test.ts", AdapterID: "typescript", Blocks: []assemblyline.SourceBlock{{ID: "tests.runtime", Static: "void 0;", API: "runtime test", DependsOn: []string{"runtime.factory"}}}},
	)
	return assemblyline.SourceBlueprint{Documents: documents}
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
	ids := make([]string, 0, len(program.Source.Documents))
	for _, document := range program.Source.Documents {
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
