package worker

import (
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
