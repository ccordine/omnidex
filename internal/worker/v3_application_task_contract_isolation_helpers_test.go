package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func isolatedTaskContractSpecification(
	surface assemblyline.ApplicationSurface,
) assemblyline.ApplicationSpecification {
	return assemblyline.ApplicationSpecification{
		Surface: surface, ProductQuote: "environmental sensor console",
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
			if strings.Count(block.Contract, own) != 1 ||
				strings.Count(block.Contract, specification.ProductQuote) != 1 ||
				strings.Count(block.Contract, "Authoritative product context:") != 1 ||
				strings.Contains(block.Contract, sibling) {
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
