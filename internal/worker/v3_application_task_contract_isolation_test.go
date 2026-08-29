package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestBrowserAndCommandLineCompilersKeepOneRequirementPerGeneratedContract(t *testing.T) {
	t.Parallel()
	for _, surface := range []assemblyline.ApplicationSurface{
		assemblyline.ApplicationSurfaceBrowser,
		assemblyline.ApplicationSurfaceCommandLine,
	} {
		surface := surface
		t.Run(string(surface), func(t *testing.T) {
			t.Parallel()
			specification := isolatedTaskContractSpecification(surface)
			workload, err := assemblyline.FreezeApplicationWorkload(specification)
			if err != nil {
				t.Fatal(err)
			}
			contexts, err := directCodingApplicationTaskContexts(workload)
			if err != nil {
				t.Fatal(err)
			}
			coverage := isolatedTaskContractCoverage(workload, surface)
			capabilities := directCodingCapabilityGraph{
				"requirement_001": nil,
				"requirement_002": nil,
			}
			documents, err := isolatedTaskContractDocuments(
				surface, specification, contexts, capabilities, coverage,
			)
			if err != nil {
				t.Fatal(err)
			}
			assertIsolatedTaskContracts(t, specification, documents)
		})
	}
}

func TestBrowserAcceptanceContractRequiresCausalStaticInteractionEvidence(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name     string
		product  string
		behavior string
		sibling  string
	}{
		{
			name:     "inventory quantity adjustment",
			product:  "Warehouse inventory application.",
			behavior: "A user enters a quantity and submits it to see the updated inventory total.",
			sibling:  "A user can inspect the current warehouse location.",
		},
		{
			name:     "travel route duration",
			product:  "Journey planning application.",
			behavior: "A traveler supplies route values and requests the resulting journey duration.",
			sibling:  "A traveler can inspect the selected travel class.",
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			specification := assemblyline.ApplicationSpecification{
				Surface:      assemblyline.ApplicationSurfaceBrowser,
				ProductQuote: fixture.product,
				Requirements: []assemblyline.Requirement{
					{ID: "requirement_001", SourceQuote: fixture.behavior},
					{ID: "requirement_002", SourceQuote: fixture.sibling},
				},
			}
			workload, err := assemblyline.FreezeApplicationWorkload(specification)
			if err != nil {
				t.Fatal(err)
			}
			contexts, err := directCodingApplicationTaskContexts(workload)
			if err != nil {
				t.Fatal(err)
			}
			coverage := isolatedTaskContractCoverage(
				workload, assemblyline.ApplicationSurfaceBrowser,
			)
			capabilities := directCodingCapabilityGraph{
				"requirement_001": nil,
				"requirement_002": nil,
			}
			featureDocuments, err := genericBrowserFeatureDocuments(
				specification, map[string]directCodingSkillBinding{},
				contexts, capabilities, coverage,
			)
			if err != nil {
				t.Fatal(err)
			}
			acceptanceDocuments, err := genericBrowserAcceptanceDocuments(
				specification, contexts, capabilities, coverage,
			)
			if err != nil {
				t.Fatal(err)
			}
			program := directCodingProgram{
				StackID:          genericTypeScriptBrowserAdapter,
				VersionProfileID: typeScriptBrowserVersionProfileV1,
				Source: assemblyline.SourceBlueprint{Documents: append(
					append([]assemblyline.SourceDocument(nil), featureDocuments...),
					acceptanceDocuments...,
				)},
				Generated: map[string]string{},
			}
			acceptance := directCodingTestGeneratedBlockRef(t, program.Source, "acceptance.001")
			acceptanceInput, err := directCodingLanguageFragmentInput(
				&program, acceptance, "typescript",
			)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(
				acceptanceInput.PermittedSymbols,
				[]string{"fireEvent", "screen", "waitFor", "expect"},
			) {
				t.Fatalf("acceptance globals=%q", acceptanceInput.PermittedSymbols)
			}
			acceptanceJob, err := assemblyline.NewFragmentGenerationJob(acceptanceInput)
			if err != nil {
				t.Fatal(err)
			}
			acceptancePrompt, err := assemblyline.RenderPortableJob(acceptanceJob)
			if err != nil {
				t.Fatal(err)
			}
			feature := directCodingTestGeneratedBlockRef(t, program.Source, "feature.001")
			featureInput, err := directCodingLanguageFragmentInput(&program, feature, "typescript")
			if err != nil {
				t.Fatal(err)
			}
			featureJob, err := assemblyline.NewFragmentGenerationJob(featureInput)
			if err != nil {
				t.Fatal(err)
			}
			featurePrompt, err := assemblyline.RenderPortableJob(featureJob)
			if err != nil {
				t.Fatal(err)
			}

			if strings.Count(acceptancePrompt, fixture.behavior) != 1 {
				t.Fatalf("acceptance prompt does not contain its exact behavior once:\n%s", acceptancePrompt)
			}
			for _, required := range []string{
				"Realize every explicit interaction condition in the exact requirement before observing its outcome.",
				"enter concrete static values with fireEvent.change or fireEvent.input before activating the behavior",
				"Every asserted derived value must be grounded in the exact requirement",
				"exactly determined by the preceding static event payloads",
				"Do not invent an accessible control name that the exact requirement does not state",
				"assert the exact element that owns the expected text or accessible value",
			} {
				if strings.Count(acceptancePrompt, required) != 1 {
					t.Fatalf("acceptance prompt does not contain one exact causal rail %q:\n%s", required, acceptancePrompt)
				}
				if strings.Contains(featurePrompt, required) {
					t.Fatalf("implementation prompt inherited verification rail %q:\n%s", required, featurePrompt)
				}
			}
			for _, forbidden := range []string{
				fixture.product,
				fixture.sibling,
				"src/Features.tsx",
				"src/Features.test.tsx",
				"acceptance.001",
				"task_001",
			} {
				if strings.Contains(acceptancePrompt, forbidden) {
					t.Fatalf("acceptance prompt exposes unrelated authority %q:\n%s", forbidden, acceptancePrompt)
				}
			}
		})
	}
}

func isolatedTaskContractSpecification(
	surface assemblyline.ApplicationSurface,
) assemblyline.ApplicationSpecification {
	return assemblyline.ApplicationSpecification{
		Surface: surface, ProductQuote: "aggregate product context sentinel",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Display the current reading."},
			{ID: "requirement_002", SourceQuote: "Export the retained history."},
		},
	}
}

func isolatedTaskContractCoverage(
	workload assemblyline.FrozenApplicationWorkload,
	surface assemblyline.ApplicationSurface,
) assemblyline.ApplicationFileCoveragePlan {
	implementation, verification := "features.go", "features_test.go"
	if surface == assemblyline.ApplicationSurfaceBrowser {
		implementation, verification = "src/Features.tsx", "src/Features.test.tsx"
	}
	taskIDs := []string{workload.Tasks[0].ID, workload.Tasks[1].ID}
	return assemblyline.ApplicationFileCoveragePlan{
		WorkloadSHA256: workload.SHA256,
		Files: []assemblyline.ApplicationFileCoverage{
			{Path: implementation, Kind: assemblyline.TargetArtifactImplementation, TaskIDs: taskIDs},
			{Path: verification, Kind: assemblyline.TargetArtifactVerification, TaskIDs: taskIDs},
		},
	}
}

func isolatedTaskContractDocuments(
	surface assemblyline.ApplicationSurface,
	specification assemblyline.ApplicationSpecification,
	contexts map[string]assemblyline.ApplicationTaskContext,
	capabilities directCodingCapabilityGraph,
	coverage assemblyline.ApplicationFileCoveragePlan,
) ([]assemblyline.SourceDocument, error) {
	if surface == assemblyline.ApplicationSurfaceBrowser {
		implementations, err := genericBrowserFeatureDocuments(
			specification, map[string]directCodingSkillBinding{},
			contexts, capabilities, coverage,
		)
		if err != nil {
			return nil, err
		}
		verifications, err := genericBrowserAcceptanceDocuments(
			specification, contexts, capabilities, coverage,
		)
		return append(implementations, verifications...), err
	}
	return genericGoCommandLineDocuments(
		specification, map[string]directCodingSkillBinding{},
		contexts, capabilities, coverage,
	)
}

func assertIsolatedTaskContracts(
	t *testing.T,
	specification assemblyline.ApplicationSpecification,
	documents []assemblyline.SourceDocument,
) {
	t.Helper()
	implementationCount, verificationCount := 0, 0
	for _, document := range documents {
		for _, block := range document.Blocks {
			if block.Role != assemblyline.SourceBlockTaskImplementation &&
				block.Role != assemblyline.SourceBlockTaskVerification {
				continue
			}
			if block.Role == assemblyline.SourceBlockTaskImplementation {
				implementationCount++
			} else {
				verificationCount++
			}
			index := 0
			if block.TaskID == "task_002" {
				index = 1
			} else if block.TaskID != "task_001" {
				t.Fatalf("generated contract has unknown task %q", block.TaskID)
			}
			own := specification.Requirements[index].SourceQuote
			sibling := specification.Requirements[1-index].SourceQuote
			if !strings.Contains(block.Contract, own) ||
				strings.Contains(block.Contract, sibling) ||
				strings.Contains(block.Contract, specification.ProductQuote) {
				t.Fatalf("block %s contract is not task-local:\n%s", block.ID, block.Contract)
			}
		}
	}
	want := len(specification.Requirements)
	if implementationCount != want || verificationCount != want {
		t.Fatalf(
			"generated implementation/verification blocks=%d/%d want %d/%d",
			implementationCount, verificationCount, want, want,
		)
	}
}
