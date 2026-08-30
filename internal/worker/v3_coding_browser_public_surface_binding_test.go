package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestBrowserPublicSurfaceBindingProjectsOnlyAcceptedPublicFacts(t *testing.T) {
	fixtures := []struct {
		name          string
		behavior      string
		featureSource string
		private       []string
		badQuery      string
		wantFailure   string
		changedSource string
	}{
		{
			name: "inventory", behavior: "Adjust an inventory quantity.",
			featureSource: `function FeatureView(): ReactElement {
	const [privateInventoryValue, setPrivateInventoryValue] = useState("");
  return <main><label htmlFor="sku">Stock code</label><input id="sku" value={privateInventoryValue} onChange={(event) => setPrivateInventoryValue(event.target.value)} /><button type="button">Apply adjustment</button><p><span>Updated:</span><output aria-label="Updated inventory">{privateInventoryValue}</output></p></main>;
}`,
			private:     []string{"privateInventoryValue", "FeatureView", "value={"},
			badQuery:    `async function VerifyFeature(): Promise<void> { expect(screen.getByRole('textbox', { name: 'Missing stock name' })).toBeInTheDocument(); }`,
			wantFailure: "accessible name \"Missing stock name\"",
			changedSource: `function FeatureView(): ReactElement {
	const [privateInventoryValue, setPrivateInventoryValue] = useState("");
  return <main><label htmlFor="sku">Stock code</label><input id="sku" value={privateInventoryValue} onChange={(event) => setPrivateInventoryValue(event.target.value)} /><button type="button">Remove adjustment</button><p><span>Updated:</span><output aria-label="Updated inventory">{privateInventoryValue}</output></p></main>;
}`,
		},
		{
			name: "travel", behavior: "Find a journey duration.",
			featureSource: `function FeatureView(): ReactElement {
	const [privateDepartureValue, setPrivateDepartureValue] = useState("");
	const [privateArrivalValue, setPrivateArrivalValue] = useState("");
  return <main><input aria-label="Departure city" value={privateDepartureValue} onChange={(event) => setPrivateDepartureValue(event.target.value)} /><input aria-label="Arrival city" value={privateArrivalValue} onChange={(event) => setPrivateArrivalValue(event.target.value)} /><button type="button">Find routes</button><p><span>Duration:</span><output aria-label="Journey duration">{privateArrivalValue}</output></p></main>;
}`,
			private:     []string{"privateDepartureValue", "privateArrivalValue", "FeatureView"},
			badQuery:    `async function VerifyFeature(): Promise<void> { expect(screen.getByRole('textbox')).toBeInTheDocument(); }`,
			wantFailure: "matches 2 public controls",
			changedSource: `function FeatureView(): ReactElement {
	const [privateDepartureValue, setPrivateDepartureValue] = useState("");
	const [privateArrivalValue, setPrivateArrivalValue] = useState("");
  return <main><input aria-label="Origin city" value={privateDepartureValue} onChange={(event) => setPrivateDepartureValue(event.target.value)} /><input aria-label="Arrival city" value={privateArrivalValue} onChange={(event) => setPrivateArrivalValue(event.target.value)} /><button type="button">Find routes</button><p><span>Duration:</span><output aria-label="Journey duration">{privateArrivalValue}</output></p></main>;
}`,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			stage, featureRef, acceptanceRef := browserPublicSurfaceBindingFixture(
				t, fixture.behavior, fixture.featureSource,
			)
			executor := &directCodingTypeScriptProjectStageExecutor{
				publicSurfaceBindings: make(map[string]directCodingBrowserPublicSurfaceBinding),
			}
			before := renderBrowserPublicSurfaceTestJob(t, stage, featureRef, nil, nil)
			context, err := assemblyline.ProjectApplicationTaskContext(
				stage.Workload, "task_001",
			)
			if err != nil {
				t.Fatal(err)
			}
			surface, validate, err := executor.bindBrowserPublicSurface(context, stage, acceptanceRef)
			if err != nil {
				t.Fatal(err)
			}
			after := renderBrowserPublicSurfaceTestJob(t, stage, featureRef, nil, nil)
			if before != after || strings.Contains(after, "PUBLIC_INTERACTION_SURFACE:") {
				t.Fatalf("implementation prompt changed after public-surface binding:\n%s", after)
			}
			verificationPrompt := renderBrowserPublicSurfaceTestJob(
				t, stage, acceptanceRef, surface, validate,
			)
			if strings.Count(verificationPrompt, "PUBLIC_INTERACTION_SURFACE:") != 1 ||
				strings.Count(verificationPrompt, "PUBLIC_INTERACTION_SURFACE_V1") != 1 {
				t.Fatalf("verification prompt lacks one public receipt:\n%s", verificationPrompt)
			}
			for _, forbidden := range append(
				append([]string(nil), fixture.private...),
				"src/feature.tsx", "feature.impl", "acceptance.verify", "task_001",
			) {
				if strings.Contains(verificationPrompt, forbidden) {
					t.Fatalf("verification prompt leaked private authority %q:\n%s", forbidden, verificationPrompt)
				}
			}
			if err := validate(fixture.badQuery); err == nil ||
				!strings.Contains(err.Error(), fixture.wantFailure) {
				t.Fatalf("public query rejection=%v; want %q", err, fixture.wantFailure)
			}
			stage.Generated["feature.impl"] = fixture.changedSource
			if err := executor.validateTaskBrowserPublicSurface(stage, "task_001"); err == nil ||
				!strings.Contains(err.Error(), "surface drifted") {
				t.Fatalf("changed public surface was not rejected: %v", err)
			}
		})
	}
}

func TestBrowserPublicSurfaceBindingRejectsCrossTaskElementIDCollision(t *testing.T) {
	bindings := map[string]directCodingBrowserPublicSurfaceBinding{
		"task_inventory": {
			surface: directCodingBrowserPublicInteractionSurface{ElementIDs: []string{"value"}},
		},
		"task_travel": {
			surface: directCodingBrowserPublicInteractionSurface{ElementIDs: []string{"value"}},
		},
	}
	tasks := []assemblyline.FrozenApplicationTask{
		{ID: "task_inventory"}, {ID: "task_travel"},
	}
	err := validateDirectCodingBrowserPublicElementIDs(bindings, tasks)
	if err == nil || !strings.Contains(err.Error(), `repeat public element id "value"`) {
		t.Fatalf("cross-task public id collision was not rejected: %v", err)
	}
}

func TestBrowserPublicSurfaceBindingRejectsStaticMountElementIDCollision(t *testing.T) {
	bindings := map[string]directCodingBrowserPublicSurfaceBinding{
		"task_inventory": {
			surface: directCodingBrowserPublicInteractionSurface{ElementIDs: []string{"root"}},
		},
	}
	tasks := []assemblyline.FrozenApplicationTask{{ID: "task_inventory"}}
	err := validateDirectCodingBrowserPublicElementIDs(bindings, tasks)
	if err == nil || !strings.Contains(
		err.Error(), `repeats code-owned browser mount element id "root"`,
	) {
		t.Fatalf("code-owned mount id collision was not rejected: %v", err)
	}
}

func TestBrowserPublicSurfaceBindingRejectsCodeOnlyElementIDDrift(t *testing.T) {
	stage, _, acceptance := browserPublicSurfaceBindingFixture(
		t,
		"Adjust one quantity.",
		`function FeatureView(): ReactElement { return <main><label htmlFor="value">Quantity</label><input id="value" /><button type="button">Apply adjustment</button></main>; }`,
	)
	executor := &directCodingTypeScriptProjectStageExecutor{
		publicSurfaceBindings: make(map[string]directCodingBrowserPublicSurfaceBinding),
	}
	context, err := assemblyline.ProjectApplicationTaskContext(stage.Workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := executor.bindBrowserPublicSurface(context, stage, acceptance); err != nil {
		t.Fatal(err)
	}
	stage.Generated["feature.impl"] =
		`function FeatureView(): ReactElement { return <main><label htmlFor="amount">Quantity</label><input id="amount" /><button type="button">Apply adjustment</button></main>; }`
	if err := executor.validateTaskBrowserPublicSurface(stage, "task_001"); err == nil ||
		!strings.Contains(err.Error(), "surface drifted") {
		t.Fatalf("code-only public id drift was not rejected: %v", err)
	}
}

func TestBrowserPublicSurfaceBindingRejectsResultRelationReceiptDrift(t *testing.T) {
	stage, _, acceptance := browserPublicSurfaceBindingFixture(
		t,
		"Derive one visible quantity from one supplied value.",
		`function FeatureView(): ReactElement { return <main><label htmlFor="value">Quantity</label><input id="value" /><button type="button">Apply quantity</button><p>Result: 2</p></main>; }`,
	)
	executor := &directCodingTypeScriptProjectStageExecutor{
		publicSurfaceBindings: make(map[string]directCodingBrowserPublicSurfaceBinding),
	}
	context, err := assemblyline.ProjectApplicationTaskContext(stage.Workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := executor.bindBrowserPublicSurface(context, stage, acceptance); err != nil {
		t.Fatal(err)
	}
	stage.RequirementRelations.Bindings[0].Receipt.Relation =
		assemblyline.ApplicationRequirementNoDerivedResult
	if err := executor.validateTaskBrowserPublicSurface(stage, "task_001"); err == nil ||
		!strings.Contains(err.Error(), "surface drifted") {
		t.Fatalf("result-relation receipt drift was not rejected: %v", err)
	}
}

func browserPublicSurfaceBindingFixture(
	t *testing.T,
	behavior string,
	featureSource string,
) (*directCodingProgram, assemblyline.SourceBlockRef, assemblyline.SourceBlockRef) {
	t.Helper()
	workload, err := assemblyline.FreezeApplicationWorkload(
		assemblyline.ApplicationSpecification{
			Surface:      assemblyline.ApplicationSurfaceBrowser,
			ProductQuote: "neutral browser fixture",
			Requirements: []assemblyline.Requirement{
				{ID: "requirement_001", SourceQuote: behavior},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	feature := assemblyline.SourceBlock{
		ID: "feature.impl", Signature: "function FeatureView(): ReactElement",
		Contract: behavior, API: "function FeatureView(): ReactElement",
		Globals: []string{"ReactElement"}, TaskID: "task_001",
		Role: assemblyline.SourceBlockTaskImplementation,
	}
	acceptance := assemblyline.SourceBlock{
		ID: "acceptance.verify", Signature: "async function VerifyFeature(): Promise<void>",
		Contract: behavior, API: "async function VerifyFeature(): Promise<void>",
		DependsOn: []string{feature.ID}, Globals: []string{"expect", "screen"},
		TaskID: "task_001", Role: assemblyline.SourceBlockTaskVerification,
	}
	featureDocument := assemblyline.SourceDocument{
		ID: "feature_document", Path: "src/feature.tsx", AdapterID: "typescript_react",
		Blocks: []assemblyline.SourceBlock{feature},
	}
	acceptanceDocument := assemblyline.SourceDocument{
		ID: "acceptance_document", Path: "src/feature.test.tsx", AdapterID: "typescript_react",
		Blocks: []assemblyline.SourceBlock{acceptance},
	}
	stage := &directCodingProgram{
		StackID:          genericTypeScriptBrowserAdapter,
		VersionProfileID: typeScriptBrowserVersionProfileV1,
		Workload:         workload,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			featureDocument, acceptanceDocument,
		}},
		Generated: map[string]string{feature.ID: featureSource},
	}
	bindDirectCodingTestRequirementRelations(
		t, stage, assemblyline.ApplicationRequirementExplicitResultRelation,
	)
	return stage,
		assemblyline.SourceBlockRef{Document: featureDocument, Block: feature},
		assemblyline.SourceBlockRef{Document: acceptanceDocument, Block: acceptance}
}

func renderBrowserPublicSurfaceTestJob(
	t *testing.T,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	surface *assemblyline.FragmentPublicInteractionSurface,
	validate func(string) error,
) string {
	t.Helper()
	job, err := directCodingApplicationTaskFragmentJob(stage, ref, surface, validate)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := newDirectCodingTypeScriptPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(portable)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}
