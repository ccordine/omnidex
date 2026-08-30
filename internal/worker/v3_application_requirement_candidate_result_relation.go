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
	input := assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateResultRelationJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateResultRelationResult{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		"application_requirement_candidate_result_relation",
		job,
		identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidateResultRelationResult, error) {
			return assemblyline.DecodeApplicationRequirementCandidateResultRelationResult(input, raw)
		},
		func(value assemblyline.ApplicationRequirementCandidateResultRelationResult) error {
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
	generationAuthority assemblyline.ApplicationRequirementCandidateInput,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	resultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult,
	grounding assemblyline.ApplicationRequirementCandidateResultRelationGroundingResult,
	identities []assemblyline.ArtifactIdentity,
) (string, error) {
	if _, err := assemblyline.NewApplicationRequirementJob(generationAuthority); err != nil {
		return "", fmt.Errorf("validate result-relation correction request authority: %w", err)
	}
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
		ImmutableRequest: generationAuthority.Authority.UserRequest,
		Context:          generationAuthority.Authority.Context,
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
