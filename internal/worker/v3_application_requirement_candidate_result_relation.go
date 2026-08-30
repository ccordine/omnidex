package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

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
			finalInput,
			derivedValue,
			nil,
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
		finalInput,
		derivedValue,
		&determiningRelation,
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
		func(value assemblyline.ApplicationRequirementCandidateResultPresenceResult) error {
			return value.ValidateFor(input)
		},
	)
}

func groundDirectCodingApplicationRequirementCandidateResultRelation(
	runtime typedWorkerRuntime,
	intentModel string,
	immutableRequest string,
	context assemblyline.ApplicationContext,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	resultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateResultRelationGroundingResult, error) {
	input := assemblyline.ApplicationRequirementCandidateResultRelationGroundingInput{
		ImmutableRequest: immutableRequest,
		Context:          context,
		CandidateAuthority: assemblyline.ApplicationRequirementCandidateResultRelationInput{
			Candidate: candidate, Kind: kind, Cardinality: cardinality,
		},
		MissingResultRelation: resultRelation,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateResultRelationGroundingJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateResultRelationGroundingResult{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		"application_requirement_candidate_result_relation_grounding",
		job,
		identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidateResultRelationGroundingResult, error) {
			return assemblyline.DecodeApplicationRequirementCandidateResultRelationGroundingResult(
				input,
				raw,
			)
		},
		func(value assemblyline.ApplicationRequirementCandidateResultRelationGroundingResult) error {
			return value.ValidateFor(input)
		},
	)
}

func correctDirectCodingApplicationRequirementCandidateResultRelation(
	runtime typedWorkerRuntime,
	intentModel string,
	immutableRequest string,
	context assemblyline.ApplicationContext,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	resultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult,
	grounding assemblyline.ApplicationRequirementCandidateResultRelationGroundingResult,
	identities []assemblyline.ArtifactIdentity,
) (string, error) {
	candidateAuthority := assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
	if err := resultRelation.ValidateFor(candidateAuthority); err != nil {
		return "", fmt.Errorf("validate result-relation correction defect receipt: %w", err)
	}
	if resultRelation.Relation != assemblyline.ApplicationRequirementMissingResultRelation {
		return "", fmt.Errorf(
			"result-relation correction requires code-established defect %q",
			assemblyline.ApplicationRequirementMissingResultRelation,
		)
	}
	input := assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput{
		ImmutableRequest: immutableRequest,
		Context:          context,
		CurrentCandidate: candidate,
		Defect:           resultRelation.Relation,
		Grounding:        grounding,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateResultRelationCorrectionJob(input)
	if err != nil {
		return "", err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		"application_requirement_candidate_result_relation_correction",
		job,
		identities,
		func(raw string) (string, error) {
			return assemblyline.DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf(
				input, raw,
			)
		},
		func(value string) error {
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application requirement candidate result-relation correction",
				runtime.PathProvenance,
				value,
			)
		},
	)
}
