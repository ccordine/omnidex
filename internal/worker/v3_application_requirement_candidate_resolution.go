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
}

func resolveDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	candidate string,
	generationAuthority assemblyline.ApplicationRequirementCandidateInput,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationRequirementCandidateResolution, error) {
	var zero directCodingApplicationRequirementCandidateResolution
	replacementUsed := false
	resultRelationCorrectionUsed := false
	for {
		if duplicate, found := directCodingApplicationRequirementDuplicate(
			generationAuthority.Authority, candidate,
		); found {
			if replacementUsed {
				return zero, fmt.Errorf(
					"application requirement candidate remained an exact duplicate after its one replacement",
				)
			}
			replacement, err := replaceDirectCodingApplicationRequirementDuplicate(
				runtime, intentModel, generationAuthority, candidate, duplicate, identities,
			)
			if err != nil {
				return zero, err
			}
			candidate = replacement
			replacementUsed = true
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
		if _, duplicate := directCodingApplicationRequirementDuplicate(
			generationAuthority.Authority, candidate,
		); duplicate {
			continue
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
			candidate, err = correctDirectCodingApplicationRequirementCandidateResultRelation(
				runtime, intentModel, generationAuthority, candidate, kind,
				refinement.Cardinality, resultRelation, identities,
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

func replaceDirectCodingApplicationRequirementDuplicate(
	runtime typedWorkerRuntime,
	intentModel string,
	generationAuthority assemblyline.ApplicationRequirementCandidateInput,
	candidate string,
	duplicate assemblyline.ApplicationRequirementCandidateDuplicateIdentity,
	identities []assemblyline.ArtifactIdentity,
) (string, error) {
	input := assemblyline.ApplicationRequirementCandidateDuplicateReplacementInput{
		GenerationAuthority: generationAuthority,
		CurrentCandidate:    candidate,
		Duplicate:           duplicate,
		Defect:              assemblyline.ApplicationRequirementDuplicateCandidateDefect,
	}
	job, err := assemblyline.NewApplicationRequirementCandidateDuplicateReplacementJob(input)
	if err != nil {
		return "", err
	}
	replacement, err := runDirectCodingSemanticLeafCall(
		runtime, intentModel, "application_requirement_candidate_duplicate_replacement",
		job, identities,
		func(raw string) (string, error) {
			return assemblyline.DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(
				input, raw,
			)
		},
		func(value string) error {
			if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application requirement candidate duplicate replacement",
				runtime.PathProvenance, value,
			); err != nil {
				return err
			}
			if _, duplicate := directCodingApplicationRequirementDuplicate(
				generationAuthority.Authority, value,
			); duplicate {
				return fmt.Errorf(
					"application requirement duplicate replacement returned another exact duplicate",
				)
			}
			return nil
		},
	)
	if err != nil {
		return "", err
	}
	return replacement, nil
}

func directCodingApplicationRequirementDuplicate(
	authority assemblyline.ApplicationRequirementCoverageInput,
	candidate string,
) (assemblyline.ApplicationRequirementCandidateDuplicateIdentity, bool) {
	for index, accepted := range authority.AcceptedRequirements {
		if candidate == accepted {
			return assemblyline.ApplicationRequirementCandidateDuplicateIdentity{
				Set:   assemblyline.ApplicationRequirementDuplicateAcceptedRequirement,
				Index: index,
			}, true
		}
	}
	for index, excluded := range authority.ExcludedCandidates {
		if candidate == excluded {
			return assemblyline.ApplicationRequirementCandidateDuplicateIdentity{
				Set:   assemblyline.ApplicationRequirementDuplicateExcludedNonRuntimeCandidate,
				Index: index,
			}, true
		}
	}
	return assemblyline.ApplicationRequirementCandidateDuplicateIdentity{}, false
}
