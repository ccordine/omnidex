package worker

import "github.com/gryph/omnidex/internal/assemblyline"

type directCodingApplicationRequirementExactDuplicateKind uint8

const (
	directCodingApplicationRequirementNotExactDuplicate directCodingApplicationRequirementExactDuplicateKind = iota
	directCodingApplicationRequirementExactAcceptedDuplicate
	directCodingApplicationRequirementExactProcessedDuplicate
)

func directCodingApplicationRequirementExactDuplicate(
	candidate string,
	acceptedRequirements []assemblyline.ApplicationIntentCandidateRequirement,
	processedCandidates []string,
) directCodingApplicationRequirementExactDuplicateKind {
	for _, accepted := range acceptedRequirements {
		if candidate == accepted.Statement {
			return directCodingApplicationRequirementExactAcceptedDuplicate
		}
	}
	for _, processed := range processedCandidates {
		if candidate == processed {
			return directCodingApplicationRequirementExactProcessedDuplicate
		}
	}
	return directCodingApplicationRequirementNotExactDuplicate
}

func directCodingApplicationRequirementSemanticDuplicate(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	acceptedRequirements []assemblyline.ApplicationIntentCandidateRequirement,
	identities []assemblyline.ArtifactIdentity,
) (bool, error) {
	for _, accepted := range acceptedRequirements {
		input := assemblyline.ApplicationRequirementCandidateOutcomeRelationInput{
			Candidate: candidate, Kind: kind, Cardinality: cardinality,
			AcceptedRequirement:    accepted.Statement,
			AcceptedResultRelation: accepted.ResultRelation,
		}
		job, err := assemblyline.NewApplicationRequirementCandidateOutcomeRelationJob(input)
		if err != nil {
			return false, err
		}
		relation, err := runDirectCodingSemanticLeafCall(
			runtime,
			intentModel,
			"application_requirement_candidate_outcome_relation",
			job,
			identities,
			func(raw string) (assemblyline.ApplicationRequirementCandidateOutcomeRelationResult, error) {
				return assemblyline.DecodeApplicationRequirementCandidateOutcomeRelationResult(input, raw)
			},
		)
		if err != nil {
			return false, err
		}
		if relation.Relation == assemblyline.ApplicationRequirementSameRuntimeOutcome {
			return true, nil
		}
	}
	return false, nil
}
