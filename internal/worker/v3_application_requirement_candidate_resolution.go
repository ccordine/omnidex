package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationRequirementCandidateResolution struct {
	Candidate                  string
	Retain                     bool
	ResultRelation             assemblyline.ApplicationRequirementCandidateResultRelationResult
	ReboundGenerationAuthority *assemblyline.ApplicationRequirementCandidateInput
	ZeroDelta                  *assemblyline.ApplicationRequirementCandidateZeroDelta
}

func resolveDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	generationAuthority assemblyline.ApplicationRequirementCandidateInput,
	acceptedRequirements []assemblyline.ApplicationIntentCandidateRequirement,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationRequirementCandidateResolution, error) {
	var zero directCodingApplicationRequirementCandidateResolution
	resultRelationCorrectionUsed := false
	for {
		zeroDelta, found, err := directCodingApplicationRequirementExactZeroDelta(
			generationAuthority.Authority, candidate,
		)
		if err != nil {
			return zero, err
		}
		if found {
			return directCodingApplicationRequirementCandidateResolution{
				Candidate: candidate, ZeroDelta: &zeroDelta,
			}, nil
		}

		kind, err := classifyDirectCodingApplicationRequirementCandidate(
			runtime, intentModel, candidate, identities,
		)
		if err != nil {
			return zero, err
		}
		if kind.Relation == assemblyline.ApplicationRequirementCandidateNonRuntime {
			rebound, err := assemblyline.RebindApplicationRequirementAfterNonRuntimeExclusion(
				generationAuthority, candidate, kind,
			)
			if err != nil {
				return zero, err
			}
			return directCodingApplicationRequirementCandidateResolution{
				Candidate: candidate, ReboundGenerationAuthority: &rebound,
			}, nil
		}

		refinement, err := refineDirectCodingApplicationRequirementCandidate(
			runtime, intentModel, candidate, generationAuthority.Authority, identities,
		)
		if err != nil {
			return zero, err
		}
		classifiedCandidate := candidate
		candidate = refinement.Candidate
		zeroDelta, found, err = directCodingApplicationRequirementExactZeroDelta(
			generationAuthority.Authority, candidate,
		)
		if err != nil {
			return zero, err
		}
		if found {
			return directCodingApplicationRequirementCandidateResolution{
				Candidate: candidate, ZeroDelta: &zeroDelta,
			}, nil
		}
		if candidate != classifiedCandidate {
			kind, err = classifyDirectCodingApplicationRequirementCandidate(
				runtime, intentModel, candidate, identities,
			)
			if err != nil {
				return zero, err
			}
			if kind.Relation == assemblyline.ApplicationRequirementCandidateNonRuntime {
				rebound, err := assemblyline.RebindApplicationRequirementAfterNonRuntimeExclusion(
					generationAuthority, candidate, kind,
				)
				if err != nil {
					return zero, err
				}
				return directCodingApplicationRequirementCandidateResolution{
					Candidate: candidate, ReboundGenerationAuthority: &rebound,
				}, nil
			}
		}
		semanticZeroDelta, found, err := directCodingApplicationRequirementSemanticZeroDelta(
			runtime, intentModel, generationAuthority.Authority, candidate, kind,
			refinement.Cardinality, acceptedRequirements, identities,
		)
		if err != nil {
			return zero, err
		}
		if found {
			return directCodingApplicationRequirementCandidateResolution{
				Candidate: candidate, ZeroDelta: &semanticZeroDelta,
			}, nil
		}
		resultRelation, err := classifyDirectCodingApplicationRequirementCandidateResultRelation(
			runtime, intentModel, candidate, kind, refinement.Cardinality, identities,
		)
		if err != nil {
			return zero, err
		}
		if resultRelation.Relation == assemblyline.ApplicationRequirementMissingResultRelation {
			if resultRelationCorrectionUsed {
				return zero, fmt.Errorf(
					"application requirement candidate result relation remained underdetermined after its one correction",
				)
			}
			grounding, groundingErr := groundDirectCodingApplicationRequirementCandidateResultRelation(
				runtime,
				intentModel,
				generationAuthority.Authority.UserRequest,
				generationAuthority.Authority.Context,
				candidate,
				kind,
				refinement.Cardinality,
				resultRelation,
				identities,
			)
			if groundingErr != nil {
				return zero, groundingErr
			}
			if grounding.Relation != assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed {
				return zero, fmt.Errorf(
					"application requirement candidate requires a derived result, but the immutable request does not entail exactly one determining relation; refusing to invent result semantics",
				)
			}
			candidate, err = correctDirectCodingApplicationRequirementCandidateResultRelation(
				runtime, intentModel, generationAuthority, candidate, kind,
				refinement.Cardinality, resultRelation, grounding, identities,
			)
			if err != nil {
				return zero, err
			}
			resultRelationCorrectionUsed = true
			continue
		}
		if err := resultRelation.ValidateAcceptedFor(candidate); err != nil {
			return zero, err
		}
		return directCodingApplicationRequirementCandidateResolution{
			Candidate: candidate, Retain: true, ResultRelation: resultRelation,
		}, nil
	}
}

func classifyDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidateKindResult, error) {
	input := assemblyline.ApplicationRequirementCandidateKindInput{Candidate: candidate}
	job, err := assemblyline.NewApplicationRequirementCandidateKindJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidateKindResult{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime, intentModel, "application_requirement_candidate_kind", job, identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidateKindResult, error) {
			return assemblyline.DecodeApplicationRequirementCandidateKindResult(input, raw)
		},
		func(value assemblyline.ApplicationRequirementCandidateKindResult) error {
			return value.ValidateFor(input)
		},
	)
}
