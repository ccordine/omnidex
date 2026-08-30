package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func classifyDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateKindResult, bool, error) {
	runtimeContent, err := classifyDirectCodingApplicationRequirementCandidateContentPresence(
		runtime,
		intentModel,
		candidate,
		assemblyline.ApplicationRequirementCandidateRuntimeContentDimension,
		"application_requirement_candidate_runtime_content_presence",
		identities,
	)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateKindResult{}, false, err
	}
	if runtimeContent.Presence == assemblyline.ApplicationRequirementCandidateContentAbsent {
		return assemblyline.ApplicationRequirementCandidateKindResult{}, false, nil
	}
	nonRuntimeContent, err := classifyDirectCodingApplicationRequirementCandidateContentPresence(
		runtime,
		intentModel,
		candidate,
		assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension,
		"application_requirement_candidate_non_runtime_content_presence",
		identities,
	)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateKindResult{}, false, err
	}
	return assemblyline.ResolveApplicationRequirementCandidateKind(
		candidate,
		runtimeContent,
		nonRuntimeContent,
	)
}

func classifyDirectCodingApplicationRequirementCandidateContentPresence(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	dimension assemblyline.ApplicationRequirementCandidateContentDimension,
	subject string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateContentPresenceResult, error) {
	input := assemblyline.ApplicationRequirementCandidateContentPresenceInput{
		Candidate: candidate,
		Dimension: dimension,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateContentPresenceJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateContentPresenceResult{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		subject,
		job,
		identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidateContentPresenceResult, error) {
			return assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult(input, raw)
		},
		func(value assemblyline.ApplicationRequirementCandidateContentPresenceResult) error {
			return value.ValidateFor(input)
		},
	)
}
