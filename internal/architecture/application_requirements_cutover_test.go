package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationFrontDoorCutoverRemovesQuoteGateAndIterativePartitionSource(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, name := range []string{
		"internal/assemblyline/requirement_candidates.go",
		"internal/assemblyline/requirement_complete.go",
		"internal/assemblyline/requirement_projection.go",
		"internal/assemblyline/requirement_quote_validation.go",
		"internal/assemblyline/requirement_residual.go",
		"internal/assemblyline/application_requirements.go",
		"internal/assemblyline/application_requirements_portable.go",
		"internal/worker/v3_coding_requirement_partition.go",
		"internal/worker/application_requirement_single_call_test.go",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("retired iterative requirement source still exists: %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect retired requirement source %s: %v", name, err)
		}
	}

	forbidden := []string{
		"WorkApplicationIdentity",
		"WorkRequirementPartition",
		"NewApplicationIdentityJob",
		"NewRequirementPartitionJob",
		"ApplicationIdentityInput",
		"CodingProductIdentity",
		"CodingRequirementPartition",
		"coding_product_identity_model",
		"coding_requirement_partition_model",
		"RequirementPartitionInput",
		"RequirementPartitionDecision",
		"RequirementPartitionOutcome",
		"RequirementPartitionSchemaV1",
		"RequirementPartitionStepSchemaV1",
		"RequirementPartitionMode",
		"RequirementExtractFeatures",
		"RequirementSplitFeature",
		"RequirementPartitionStepDecision",
		"RequirementExtractionSelection",
		"RequirementExtractionCandidate",
		"RequirementPartitionNone",
		"CompleteRequirementPartition",
		"maxCompleteRequirementPartitionCalls",
		"extractRequirementQuotes",
		"orderedRequirementExclusions",
		"partitionCodingRequirements",
		"requirementExtractionCandidates",
		"BuildRequirementResidual",
		"WorkApplicationRequirements",
		"ApplicationRequirementInterpretation",
		"ApplicationRequirementInterpretationInput",
		"ApplicationRequirementInterpretationSchemaV1",
		"ResolveApplicationRequirements",
		"NewApplicationRequirementInterpretationJob",
		"DecodeApplicationRequirementInterpretationResult",
		"omnidex.application-requirements.v1",
	}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("retired iterative requirement token %q remains in %s", token, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
