package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingApplicationRequirementExactZeroDelta(
	authority assemblyline.ApplicationRequirementCoverageInput,
	candidate string,
) (assemblyline.ApplicationRequirementCandidateZeroDelta, bool, error) {
	for _, retained := range authority.ZeroDeltas {
		if candidate == retained.Candidate {
			return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, fmt.Errorf(
				"application requirement candidate repeats a recorded zero delta",
			)
		}
	}
	for index, accepted := range authority.AcceptedRequirements {
		if candidate != accepted {
			continue
		}
		return assemblyline.ApplicationRequirementCandidateZeroDelta{
			Candidate: candidate, RetainedSet: assemblyline.ApplicationRequirementZeroDeltaAcceptedSet,
			RetainedIndex: index,
		}, true, nil
	}
	for index, excluded := range authority.ExcludedCandidates {
		if candidate == excluded {
			return assemblyline.ApplicationRequirementCandidateZeroDelta{
				Candidate: candidate, RetainedSet: assemblyline.ApplicationRequirementZeroDeltaExcludedSet,
				RetainedIndex: index,
			}, true, nil
		}
	}
	return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, nil
}

func directCodingApplicationRequirementSemanticZeroDelta(
	runtime typedWorkerRuntime,
	intentModel string,
	authority assemblyline.ApplicationRequirementCoverageInput,
	candidate string,
	kind assemblyline.ApplicationRequirementCandidateKindResult,
	cardinality assemblyline.ApplicationRequirementCandidateCardinalityResult,
	acceptedRequirements []assemblyline.ApplicationIntentCandidateRequirement,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateZeroDelta, bool, error) {
	if len(acceptedRequirements) != len(authority.AcceptedRequirements) {
		return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, fmt.Errorf(
			"accepted requirement outcome authority count differs from generation authority",
		)
	}
	for index, accepted := range authority.AcceptedRequirements {
		acceptedAuthority := acceptedRequirements[index]
		if acceptedAuthority.Statement != accepted {
			return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, fmt.Errorf(
				"accepted requirement outcome authority %d differs from generation authority",
				index,
			)
		}
		input := assemblyline.ApplicationRequirementCandidateOutcomeRelationInput{
			Candidate: candidate, Kind: kind, Cardinality: cardinality,
			AcceptedRequirement:    accepted,
			AcceptedResultRelation: acceptedAuthority.ResultRelation,
		}
		job, err := assemblyline.NewApplicationRequirementCandidateOutcomeRelationJob(input)
		if err != nil {
			return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, err
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
			func(value assemblyline.ApplicationRequirementCandidateOutcomeRelationResult) error {
				return value.ValidateFor(input)
			},
		)
		if err != nil {
			return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, err
		}
		if relation.Relation == assemblyline.ApplicationRequirementSameRuntimeOutcome {
			return assemblyline.ApplicationRequirementCandidateZeroDelta{
				Candidate:              candidate,
				RetainedSet:            assemblyline.ApplicationRequirementZeroDeltaAcceptedSet,
				RetainedIndex:          index,
				CandidateKind:          kind,
				CandidateCardinality:   cardinality,
				AcceptedResultRelation: acceptedAuthority.ResultRelation,
				OutcomeRelation:        relation,
			}, true, nil
		}
	}
	return assemblyline.ApplicationRequirementCandidateZeroDelta{}, false, nil
}
