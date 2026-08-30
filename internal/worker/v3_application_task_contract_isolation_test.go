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
				Generated: map[string]string{
					"feature.001":    "IMPLEMENTATION_SOURCE_MUST_NOT_REACH_PROMPTS",
					"acceptance.001": "VERIFICATION_SOURCE_MUST_NOT_REACH_PROMPTS",
				},
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
			for _, promptCase := range []struct {
				label  string
				prompt string
			}{
				{label: "implementation", prompt: featurePrompt},
				{label: "verification", prompt: acceptancePrompt},
			} {
				if strings.Count(promptCase.prompt, fixture.product) != 1 ||
					strings.Count(promptCase.prompt, "Product context:") != 1 {
					t.Fatalf("%s prompt does not contain its exact product identity once:\n%s", promptCase.label, promptCase.prompt)
				}
				for _, forbidden := range []string{
					fixture.sibling,
					"src/Features.tsx",
					"src/Features.test.tsx",
					"feature.001",
					"acceptance.001",
					"task_001",
					"IMPLEMENTATION_SOURCE_MUST_NOT_REACH_PROMPTS",
					"VERIFICATION_SOURCE_MUST_NOT_REACH_PROMPTS",
				} {
					if strings.Contains(promptCase.prompt, forbidden) {
						t.Fatalf("%s prompt exposes unrelated authority %q:\n%s", promptCase.label, forbidden, promptCase.prompt)
					}
				}
			}
			for _, required := range []string{
				"Realize every explicit interaction condition before observing its outcome.",
				"Enter concrete static user values with fireEvent.change or fireEvent.input before activation",
				"An action name is only a selector claim",
				"Derive the unique expected result independently from the exact requirement relation and static payloads.",
				"output names locate nodes but never supply results",
				"Receipt accessible_name literals are the only named selectors.",
				"all-query indexes are zero-based",
				"getByText and findByText never prove output ownership.",
			} {
				if strings.Count(acceptancePrompt, required) != 1 {
					t.Fatalf("acceptance prompt does not contain one exact causal rail %q:\n%s", required, acceptancePrompt)
				}
				if strings.Contains(featurePrompt, required) {
					t.Fatalf("implementation prompt inherited verification rail %q:\n%s", required, featurePrompt)
				}
			}
			const publicOperationRail = "Give every requirement-defined result operation a literal accessible control name."
			if strings.Count(featurePrompt, publicOperationRail) != 1 ||
				strings.Contains(acceptancePrompt, publicOperationRail) {
				t.Fatalf("public operation rail crossed task roles:\nfeature=%s\nacceptance=%s", featurePrompt, acceptancePrompt)
			}
		})
	}
}
