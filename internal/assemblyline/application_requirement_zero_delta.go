package assemblyline

import "fmt"

const (
	ApplicationRequirementZeroDeltaAcceptedSet = "ACCEPTED_REQUIREMENT"
	ApplicationRequirementZeroDeltaExcludedSet = "EXCLUDED_NON_RUNTIME_CANDIDATE"
	MaxApplicationRequirementZeroDeltas        = MaxApplicationRequirementLeaves
)

type ApplicationRequirementCandidateZeroDelta struct {
	Candidate              string                                               `json:"candidate"`
	RetainedSet            string                                               `json:"retained_set"`
	RetainedIndex          int                                                  `json:"retained_index"`
	CandidateKind          ApplicationRequirementCandidateKindResult            `json:"candidate_kind"`
	CandidateCardinality   ApplicationRequirementCandidateCardinalityResult     `json:"candidate_cardinality"`
	AcceptedResultRelation ApplicationRequirementCandidateResultRelationResult  `json:"accepted_result_relation"`
	OutcomeRelation        ApplicationRequirementCandidateOutcomeRelationResult `json:"outcome_relation"`
}

func validateApplicationRequirementZeroDeltas(
	input ApplicationRequirementCoverageInput,
) error {
	if input.ZeroDeltas == nil {
		return fmt.Errorf("application requirement coverage requires a non-nil zero-delta set")
	}
	if len(input.ZeroDeltas) > MaxApplicationRequirementZeroDeltas {
		return fmt.Errorf(
			"application requirement coverage exceeds %d zero-delta candidates",
			MaxApplicationRequirementZeroDeltas,
		)
	}
	seen := make(map[string]struct{}, len(input.ZeroDeltas))
	for index, evidence := range input.ZeroDeltas {
		if err := evidence.validateFor(input); err != nil {
			return fmt.Errorf("application requirement zero delta %d: %w", index, err)
		}
		if _, duplicate := seen[evidence.Candidate]; duplicate {
			return fmt.Errorf("application requirement zero delta %d is duplicated", index)
		}
		seen[evidence.Candidate] = struct{}{}
	}
	return nil
}

func (evidence ApplicationRequirementCandidateZeroDelta) validateFor(
	input ApplicationRequirementCoverageInput,
) error {
	if err := validateApplicationIntentText(
		"zero-delta candidate", evidence.Candidate, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if retainedSet, retainedIndex, exact := exactApplicationRequirementRetainedIdentity(
		input, evidence.Candidate,
	); exact {
		if evidence.RetainedSet != retainedSet || evidence.RetainedIndex != retainedIndex {
			return fmt.Errorf(
				"exact zero delta requires retained identity %s at zero-based index %d",
				retainedSet,
				retainedIndex,
			)
		}
		if evidence.CandidateKind != (ApplicationRequirementCandidateKindResult{}) ||
			evidence.CandidateCardinality != (ApplicationRequirementCandidateCardinalityResult{}) ||
			evidence.AcceptedResultRelation != (ApplicationRequirementCandidateResultRelationResult{}) ||
			evidence.OutcomeRelation != (ApplicationRequirementCandidateOutcomeRelationResult{}) {
			return fmt.Errorf("exact zero delta must not carry semantic receipts")
		}
		return nil
	}
	switch evidence.RetainedSet {
	case ApplicationRequirementZeroDeltaAcceptedSet:
		if evidence.RetainedIndex < 0 ||
			evidence.RetainedIndex >= len(input.AcceptedRequirements) {
			return fmt.Errorf("accepted retained index %d is outside authority", evidence.RetainedIndex)
		}
		accepted := input.AcceptedRequirements[evidence.RetainedIndex]
		relationInput := ApplicationRequirementCandidateOutcomeRelationInput{
			Candidate: evidence.Candidate, AcceptedRequirement: accepted,
			Kind: evidence.CandidateKind, Cardinality: evidence.CandidateCardinality,
			AcceptedResultRelation: evidence.AcceptedResultRelation,
		}
		if err := evidence.OutcomeRelation.ValidateFor(relationInput); err != nil {
			return fmt.Errorf("validate accepted zero-delta outcome relation: %w", err)
		}
		if evidence.OutcomeRelation.Relation != ApplicationRequirementSameRuntimeOutcome {
			return fmt.Errorf(
				"accepted zero delta requires relation %q",
				ApplicationRequirementSameRuntimeOutcome,
			)
		}
	case ApplicationRequirementZeroDeltaExcludedSet:
		if evidence.RetainedIndex < 0 ||
			evidence.RetainedIndex >= len(input.ExcludedCandidates) {
			return fmt.Errorf("excluded retained index %d is outside authority", evidence.RetainedIndex)
		}
		return fmt.Errorf("excluded zero delta is not byte-identical to retained authority")
	default:
		return fmt.Errorf(
			"application requirement zero-delta retained set %q is not registered",
			evidence.RetainedSet,
		)
	}
	return nil
}

func exactApplicationRequirementRetainedIdentity(
	input ApplicationRequirementCoverageInput,
	candidate string,
) (string, int, bool) {
	for index, accepted := range input.AcceptedRequirements {
		if candidate == accepted {
			return ApplicationRequirementZeroDeltaAcceptedSet, index, true
		}
	}
	for index, excluded := range input.ExcludedCandidates {
		if candidate == excluded {
			return ApplicationRequirementZeroDeltaExcludedSet, index, true
		}
	}
	return "", 0, false
}

func RecordApplicationRequirementCandidateZeroDelta(
	input ApplicationRequirementCandidateInput,
	evidence ApplicationRequirementCandidateZeroDelta,
) (ApplicationRequirementCoverageInput, error) {
	var zero ApplicationRequirementCoverageInput
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(input.Authority.ZeroDeltas) == MaxApplicationRequirementZeroDeltas {
		return zero, fmt.Errorf(
			"application requirement zero deltas reached the code-owned %d-item bound",
			MaxApplicationRequirementZeroDeltas,
		)
	}
	if err := evidence.validateFor(input.Authority); err != nil {
		return zero, err
	}
	for _, retained := range input.Authority.ZeroDeltas {
		if retained.Candidate == evidence.Candidate {
			return zero, fmt.Errorf("application requirement zero delta was already recorded")
		}
	}
	authority := input.Authority
	authority.AcceptedRequirements = append([]string{}, input.Authority.AcceptedRequirements...)
	authority.ExcludedCandidates = append([]string{}, input.Authority.ExcludedCandidates...)
	authority.ZeroDeltas = append(
		append([]ApplicationRequirementCandidateZeroDelta{}, input.Authority.ZeroDeltas...),
		evidence,
	)
	if err := authority.validate(); err != nil {
		return zero, fmt.Errorf("validate zero-delta application requirement authority: %w", err)
	}
	return authority, nil
}
