package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExpectedPortableStationMaxOutputTokensIncludesExactStopReserve(t *testing.T) {
	t.Parallel()
	request := "Build a small browser tool."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	coverageInput := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: context,
		AcceptedRequirements: []string{}, ExcludedCandidates: []string{},
		ZeroDeltas: []assemblyline.ApplicationRequirementCandidateZeroDelta{},
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		coverageInput, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirement, err := assemblyline.NewApplicationRequirementJob(
		assemblyline.ApplicationRequirementCandidateInput{
			Authority: coverageInput, Coverage: coverage,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	requirementBytes, err := assemblyline.PortableResponseMaximumBytesForJob(requirement)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedPortableStationMaxOutputTokens(requirement, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if got != requirementBytes+10 {
		t.Fatalf("multiline output ceiling=%d want decoder bytes %d + ChatML reserve 10", got, requirementBytes)
	}

	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: request},
	)
	if err != nil {
		t.Fatal(err)
	}
	classificationBytes, err := assemblyline.PortableResponseMaximumBytesForJob(classification)
	if err != nil {
		t.Fatal(err)
	}
	got, err = ExpectedPortableStationMaxOutputTokens(classification, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if got != classificationBytes+1 {
		t.Fatalf("single-line output ceiling=%d want decoder bytes %d + LF reserve 1", got, classificationBytes)
	}
}

func TestExpectedPortableStationMaxOutputTokensNeverExceedsContext(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Explain the result.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedPortableStationMaxOutputTokens(job, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 {
		t.Fatalf("context-clamped output ceiling=%d want 8192", got)
	}
}

func portableStationTestMaxOutputTokens(
	t *testing.T,
	job assemblyline.PortableJob,
	contextTokens int,
) int {
	t.Helper()
	maximum, err := ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		t.Fatal(err)
	}
	return maximum
}
