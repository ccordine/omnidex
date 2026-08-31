package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func classifyDirectCodingApplicationRequirementCandidateCardinality(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateCardinalityResult, error) {
	input := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: candidate,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateCardinalityJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateCardinalityResult{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		"application_requirement_candidate_cardinality",
		job,
		identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidateCardinalityResult, error) {
			return assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(input, raw)
		},
	)
}
