package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingRequirementRejectsRecordedZeroDeltaWithoutInference(t *testing.T) {
	t.Parallel()
	const duplicate = "Display the current status."
	current := directCodingRequirementGenerationAuthorityFixture(
		t, []string{duplicate}, []string{},
	)
	rebound, err := assemblyline.RecordApplicationRequirementCandidateZeroDelta(
		current,
		assemblyline.ApplicationRequirementCandidateZeroDelta{
			Candidate: duplicate, RetainedSet: assemblyline.ApplicationRequirementZeroDeltaAcceptedSet,
			RetainedIndex: 0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		rebound, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := assemblyline.ApplicationRequirementCandidateInput{
		Authority: rebound, Coverage: coverage,
	}
	semanticCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
			semanticCalls++
			return assemblyline.PortableResult{}, nil
		},
	}
	_, err = resolveDirectCodingApplicationRequirementCandidate(
		runtime,
		"intent-model",
		duplicate,
		authority,
		directCodingAcceptedRequirementAuthorities(t, []string{duplicate}),
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "repeats a recorded zero delta") {
		t.Fatalf("recorded zero-delta repeat error=%v", err)
	}
	if semanticCalls != 0 {
		t.Fatalf("recorded zero-delta repeat dispatched %d semantic calls", semanticCalls)
	}
}
