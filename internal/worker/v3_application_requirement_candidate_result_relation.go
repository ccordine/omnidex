package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func classifyDirectCodingApplicationRequirementCandidateResultRelation(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateResultRelationResult, error) {
	finalInput := assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
	derivedValue, err := classifyDirectCodingApplicationRequirementCandidateResultPresence(
		runtime,
		intentModel,
		finalInput,
		assemblyline.ApplicationRequirementDerivedValueDimension,
		nil,
		"application_requirement_candidate_derived_value_presence",
		identities,
	)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateResultRelationResult{}, err
	}
	if derivedValue.Presence == assemblyline.ApplicationRequirementCandidateResultAbsent {
		return assemblyline.ResolveApplicationRequirementCandidateResultRelation(
			finalInput, derivedValue, nil,
		)
	}
	determiningRelation, err := classifyDirectCodingApplicationRequirementCandidateResultPresence(
		runtime,
		intentModel,
		finalInput,
		assemblyline.ApplicationRequirementDeterminingRelationDimension,
		&derivedValue,
		"application_requirement_candidate_determining_relation_presence",
		identities,
	)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateResultRelationResult{}, err
	}
	return assemblyline.ResolveApplicationRequirementCandidateResultRelation(
		finalInput, derivedValue, &determiningRelation,
	)
}

func classifyDirectCodingApplicationRequirementCandidateResultPresence(
	runtime typedWorkerRuntime,
	intentModel string,
	finalInput assemblyline.ApplicationRequirementCandidateResultRelationInput,
	dimension assemblyline.ApplicationRequirementCandidateResultDimension,
	derivedValuePresence *assemblyline.ApplicationRequirementCandidateResultPresenceResult,
	subject string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateResultPresenceResult, error) {
	input := assemblyline.ApplicationRequirementCandidateResultPresenceInput{
		Candidate: finalInput.Candidate, Kind: finalInput.Kind,
		Cardinality: finalInput.Cardinality, Dimension: dimension,
		DerivedValuePresence: derivedValuePresence,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateResultPresenceJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateResultPresenceResult{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		subject,
		job,
		identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidateResultPresenceResult, error) {
			return assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(input, raw)
		},
	)
}
