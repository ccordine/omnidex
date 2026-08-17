package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestOneTreeLeafGroupsSeveralBoundedBlocks(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "counter application",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "display a current count"},
			{ID: "requirement_002", SourceQuote: "increment the current count"},
		},
	}
	input := applicationWorkloadInput(specification)
	workload, err := assemblyline.FreezeApplicationWorkload(input, assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{
			{RequirementID: "requirement_001", Objective: "Implement count display.", RequiredBehaviors: []string{"Show the current count."}, AcceptanceCriteria: []string{"The current count is visible."}},
			{RequirementID: "requirement_002", Objective: "Implement count increment.", RequiredBehaviors: []string{"Provide an increment control."}, AcceptanceCriteria: []string{"Activating the control increases the count."}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := directCodingApplicationTaskContexts(input, workload)
	if err != nil {
		t.Fatal(err)
	}
	tree := assemblyline.TargetTree{Bindings: []assemblyline.TargetTreeRequirementBinding{
		{Path: "src/counter.tsx", Kind: assemblyline.TargetArtifactImplementation, RequirementIDs: []string{"requirement_001", "requirement_002"}},
		{Path: "tests/counter.test.tsx", Kind: assemblyline.TargetArtifactVerification, RequirementIDs: []string{"requirement_001", "requirement_002"}},
	}}
	capabilities := directCodingCapabilityGraph{"requirement_001": nil, "requirement_002": nil}
	features, err := genericBrowserFeatureDocuments(specification, map[string]directCodingSkillBinding{}, contexts, capabilities, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(features) != 1 || features[0].Path != "src/counter.tsx" || len(features[0].Blocks) != 6 {
		t.Fatalf("features=%+v", features)
	}
	acceptance, err := genericBrowserAcceptanceDocuments(specification, contexts, capabilities, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(acceptance) != 1 || acceptance[0].Path != "tests/counter.test.tsx" || len(acceptance[0].Blocks) != 6 {
		t.Fatalf("acceptance=%+v", acceptance)
	}
}
