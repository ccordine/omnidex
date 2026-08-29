package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type portableRendererV8CurrentRequirementJob struct {
	Job            assemblyline.PortableJob
	ContractSuffix string
}

func portableRendererV8CurrentRequirementJobs(
	t *testing.T,
) map[string]portableRendererV8CurrentRequirementJob {
	t.Helper()
	const request = "Create a browser status board that displays one current status."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: context,
		AcceptedRequirements: []string{}, ExcludedCandidates: []string{},
	}
	coverageJob, err := assemblyline.NewApplicationRequirementCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		input, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	requirementJob, err := assemblyline.NewApplicationRequirementJob(
		assemblyline.ApplicationRequirementCandidateInput{
			Authority: input, Coverage: coverage,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]portableRendererV8CurrentRequirementJob{
		"renderer-v8-coverage": {
			Job: coverageJob, ContractSuffix: ".v2",
		},
		"renderer-v8-requirement": {
			Job: requirementJob, ContractSuffix: ".v3",
		},
	}
}

func portableRendererV8RequirementCandidateJobs(
	t *testing.T,
) map[string]assemblyline.PortableJob {
	t.Helper()
	cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Play drum pads and a keyboard.",
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput, assemblyline.ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	cardinalityJob, err := assemblyline.NewApplicationRequirementCandidateCardinalityJob(
		cardinalityInput,
	)
	if err != nil {
		t.Fatal(err)
	}
	kindJob, err := assemblyline.NewApplicationRequirementCandidateKindJob(
		assemblyline.ApplicationRequirementCandidateKindInput{
			Candidate: "Display the current status.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	splitJob, err := assemblyline.NewApplicationRequirementCandidateSplitJob(
		assemblyline.ApplicationRequirementCandidateSplitInput{
			Candidate: cardinalityInput.Candidate, Cardinality: cardinality,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	correctionJob, err := assemblyline.NewApplicationRequirementCandidateSplitCorrectionJob(
		assemblyline.ApplicationRequirementCandidateSplitCorrectionInput{
			CurrentCandidate: cardinalityInput.Candidate,
			Cardinality:      cardinality,
			Defect:           assemblyline.ApplicationRequirementUnchangedSplitDefect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	duplicateJob := portableRendererV8RequirementDuplicateReplacementJob(t)
	return map[string]assemblyline.PortableJob{
		"renderer-v8-cardinality":           cardinalityJob,
		"renderer-v8-kind":                  kindJob,
		"renderer-v8-split":                 splitJob,
		"renderer-v8-correction":            correctionJob,
		"renderer-v8-duplicate-replacement": duplicateJob,
	}
}

func portableRendererV8RequirementDuplicateReplacementJob(
	t *testing.T,
) assemblyline.PortableJob {
	t.Helper()
	const request = "Create a browser status board that displays one current status."
	const duplicate = "Display the current status."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	coverageInput := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: context,
		AcceptedRequirements: []string{duplicate}, ExcludedCandidates: []string{},
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		coverageInput, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementCandidateDuplicateReplacementJob(
		assemblyline.ApplicationRequirementCandidateDuplicateReplacementInput{
			GenerationAuthority: assemblyline.ApplicationRequirementCandidateInput{
				Authority: coverageInput, Coverage: coverage,
			},
			CurrentCandidate: duplicate,
			Duplicate: assemblyline.ApplicationRequirementCandidateDuplicateIdentity{
				Set: assemblyline.ApplicationRequirementDuplicateAcceptedRequirement, Index: 0,
			},
			Defect: assemblyline.ApplicationRequirementDuplicateCandidateDefect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return job
}
