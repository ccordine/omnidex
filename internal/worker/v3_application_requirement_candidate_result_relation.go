package worker

import (
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

func correctDirectCodingApplicationRequirementCandidateResultRelation(
	runtime typedWorkerRuntime,
	intentModel string,
	generationAuthority assemblyline.ApplicationRequirementCandidateInput,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	resultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult,
	identities []assemblyline.ArtifactIdentity,
) (string, error) {
	input := assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput{
		GenerationAuthority: generationAuthority,
		CandidateAuthority: assemblyline.ApplicationRequirementCandidateResultRelationInput{
			Candidate: candidate, Kind: kind, Cardinality: cardinality,
		},
		ResultRelation: resultRelation,
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
