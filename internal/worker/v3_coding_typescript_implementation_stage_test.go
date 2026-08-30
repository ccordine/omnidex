package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptImplementationStageExcludesVerificationAndComposes(t *testing.T) {
	source := `function FeatureView(): ReactElement {
	return <main><input aria-label="Record value" /><button type="button">Apply update</button><p>Updated: {0}</p></main>;
}`
	stage, _, _ := browserPublicSurfaceBindingFixture(t, "Apply a record update.", source)
	stage.Generated["acceptance.verify"] = `async function VerifyFeature(): Promise<void> {
  expect(screen.getByRole('button', { name: 'Apply update' })).toBeInTheDocument();
}`
	stage.Source.Documents[1].Blocks = append(stage.Source.Documents[1].Blocks,
		assemblyline.SourceBlock{
			ID: "acceptance.harness", Static: "async function RunAcceptance(): Promise<void> {}",
			API:       "async function RunAcceptance(): Promise<void>",
			DependsOn: []string{"acceptance.verify"}, TaskID: "task_001",
			Role: assemblyline.SourceBlockTaskSupport,
		},
		assemblyline.SourceBlock{
			ID: "acceptance.register", Static: "void RunAcceptance;",
			API: "registered acceptance", DependsOn: []string{"acceptance.harness"},
			TaskID: "task_001", Role: assemblyline.SourceBlockTaskSupport,
		},
	)
	before := cloneTypeScriptImplementationStageTestBlueprint(stage.Source)
	projection, err := projectDirectCodingTypeScriptImplementationStage(
		stage, "task_001", "feature.impl",
	)
	if err != nil {
		t.Fatal(err)
	}
	projection.Generated["feature.impl"] = source
	for _, document := range projection.Source.Documents {
		for _, block := range document.Blocks {
			if strings.HasPrefix(block.ID, "acceptance.") {
				t.Fatalf("implementation compiler projection retained %s", block.ID)
			}
		}
	}
	if !reflect.DeepEqual(stage.Source, before) {
		t.Fatal("implementation compiler projection mutated the accepted task stage")
	}
	if len(projection.Generated) != 1 || projection.Generated["feature.impl"] != source {
		t.Fatalf("implementation compiler projection retained generated verifier state: %#v", projection.Generated)
	}
	if _, err := composeDirectCodingSourceProgram(projection); err != nil {
		t.Fatalf("compose compiler-closed implementation projection: %v", err)
	}
	if err := validateDirectCodingBrowserPublicInteractionCandidate(
		projection.Generated["feature.impl"],
	); err != nil {
		t.Fatalf("revalidate compiler-closed public surface: %v", err)
	}
}

func TestTypeScriptImplementationClosureReturnsCompilerCorrectedSourceBeforeBinding(t *testing.T) {
	initial := `function FeatureView(): ReactElement {
	return <main><input aria-label="Profile value" /><button type="button">Apply preference</button><p>Saved: {0}</p></main>;
}`
	corrected := `function FeatureView(): ReactElement {
	return <main><input aria-label="Profile value" /><button type="button">Confirm preference</button><p>Saved: {0}</p></main>;
}`
	stage, _, acceptanceRef := browserPublicSurfaceBindingFixture(
		t, "Store one profile preference.", initial,
	)
	stage.Generated = map[string]string{}
	validationCalls := 0
	result, err := closeDirectCodingTypeScriptImplementation(
		stage, "task_001", "feature.impl", initial,
		func(
			projection *directCodingProgram,
			validators ...func(*directCodingProgram) error,
		) error {
			if len(validators) != 1 {
				t.Fatalf("compiler closure validators=%d want=1", len(validators))
			}
			if _, err := composeDirectCodingSourceProgram(*projection); err != nil {
				t.Fatalf("compose implementation before compiler correction: %v", err)
			}
			if err := validators[0](projection); err != nil {
				t.Fatalf("validate initial public surface: %v", err)
			}
			validationCalls++
			projection.Generated["feature.impl"] = corrected
			if err := validators[0](projection); err != nil {
				t.Fatalf("revalidate compiler-corrected public surface: %v", err)
			}
			validationCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result != corrected || validationCalls != 2 {
		t.Fatalf("compiler closure result=%q validations=%d", result, validationCalls)
	}
	if len(stage.Generated) != 0 {
		t.Fatalf("compiler closure mutated the caller's generated state: %#v", stage.Generated)
	}

	stage.Generated["feature.impl"] = result
	executor := &directCodingTypeScriptProjectStageExecutor{
		publicSurfaceBindings: make(map[string]directCodingBrowserPublicSurfaceBinding),
	}
	context, err := assemblyline.ProjectApplicationTaskContext(stage.Workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	portable, _, err := executor.bindBrowserPublicSurface(context, stage, acceptanceRef)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := portable.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(receipt, "Confirm preference") || strings.Contains(receipt, "Apply preference") {
		t.Fatalf("verifier bound a stale pre-compiler public surface:\n%s", receipt)
	}
}

func TestTypeScriptImplementationClosureRejectsCompilerBrokenPublicSurface(t *testing.T) {
	initial := `function FeatureView(): ReactElement {
	return <main><button type="button">Publish entry</button><p>Published: {0}</p></main>;
}`
	stage, _, _ := browserPublicSurfaceBindingFixture(t, "Publish one journal entry.", initial)
	stage.Generated = map[string]string{}
	_, err := closeDirectCodingTypeScriptImplementation(
		stage, "task_001", "feature.impl", initial,
		func(projection *directCodingProgram, _ ...func(*directCodingProgram) error) error {
			projection.Generated["feature.impl"] = `function FeatureView(): ReactElement {
  const action = "Publish entry";
	return <main><button type="button">{action}</button><p>Published: {0}</p></main>;
}`
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "exact literal button text") {
		t.Fatalf("compiler-broken public surface was not rejected: %v", err)
	}
}

func TestTypeScriptImplementationStageRejectsUnresolvedRetainedNeighbor(t *testing.T) {
	source := `function FeatureView(): ReactElement { return <button type="button">Archive record</button>; }`
	stage, _, _ := browserPublicSurfaceBindingFixture(t, "Archive one record.", source)
	stage.Source.Documents[0].Blocks = append(stage.Source.Documents[0].Blocks,
		assemblyline.SourceBlock{
			ID: "feature.neighbor", Signature: "function Neighbor(): number",
			Contract: "Return one number.", API: "function Neighbor(): number",
			DependsOn: []string{"feature.impl"}, TaskID: "task_001",
			Role: assemblyline.SourceBlockTaskRepresentation,
		},
	)
	_, err := projectDirectCodingTypeScriptImplementationStage(
		stage, "task_001", "feature.impl",
	)
	if err == nil || !strings.Contains(err.Error(), "retained unresolved generated block") {
		t.Fatalf("unresolved retained implementation neighbor was not rejected: %v", err)
	}
}

func cloneTypeScriptImplementationStageTestBlueprint(
	blueprint assemblyline.SourceBlueprint,
) assemblyline.SourceBlueprint {
	clone := assemblyline.SourceBlueprint{
		Documents: make([]assemblyline.SourceDocument, len(blueprint.Documents)),
	}
	for index, document := range blueprint.Documents {
		document.Blocks = append([]assemblyline.SourceBlock(nil), document.Blocks...)
		document.ScopedPreambles = append([]assemblyline.SourcePreamble(nil), document.ScopedPreambles...)
		clone.Documents[index] = document
	}
	return clone
}
