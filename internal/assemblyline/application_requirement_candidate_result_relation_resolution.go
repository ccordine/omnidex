package assemblyline

import "fmt"

// ResolveApplicationRequirementCandidateResultRelation folds independently
// bound binary receipts into the code-owned final three-way relation.
func ResolveApplicationRequirementCandidateResultRelation(
	input ApplicationRequirementCandidateResultRelationInput,
	derivedValue ApplicationRequirementCandidateResultPresenceResult,
	determiningRelation *ApplicationRequirementCandidateResultPresenceResult,
) (ApplicationRequirementCandidateResultRelationResult, error) {
	var zero ApplicationRequirementCandidateResultRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	derivedInput := ApplicationRequirementCandidateResultPresenceInput{
		Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
		Dimension: ApplicationRequirementDerivedValueDimension,
	}
	if err := derivedValue.ValidateFor(derivedInput); err != nil {
		return zero, fmt.Errorf("validate derived-value presence: %w", err)
	}
	var relation string
	switch derivedValue.Presence {
	case ApplicationRequirementCandidateResultAbsent:
		if determiningRelation != nil {
			return zero, fmt.Errorf("absent derived value must not carry determining-relation presence")
		}
		relation = ApplicationRequirementNoDerivedResult
	case ApplicationRequirementCandidateResultPresent:
		if determiningRelation == nil {
			return zero, fmt.Errorf("present derived value requires determining-relation presence")
		}
		determiningInput := ApplicationRequirementCandidateResultPresenceInput{
			Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
			Dimension:            ApplicationRequirementDeterminingRelationDimension,
			DerivedValuePresence: &derivedValue,
		}
		if err := determiningRelation.ValidateFor(determiningInput); err != nil {
			return zero, fmt.Errorf("validate determining-relation presence: %w", err)
		}
		if determiningRelation.Presence == ApplicationRequirementCandidateResultPresent {
			relation = ApplicationRequirementExplicitResultRelation
		} else {
			relation = ApplicationRequirementMissingResultRelation
		}
	default:
		return zero, fmt.Errorf("application requirement derived-value presence is not registered")
	}
	kindSHA256, err := applicationRequirementSemanticReceiptSHA256(input.Kind)
	if err != nil {
		return zero, fmt.Errorf("hash application requirement kind receipt: %w", err)
	}
	cardinalitySHA256, err := applicationRequirementSemanticReceiptSHA256(input.Cardinality)
	if err != nil {
		return zero, fmt.Errorf("hash application requirement cardinality receipt: %w", err)
	}
	result := ApplicationRequirementCandidateResultRelationResult{
		Schema:            ApplicationRequirementCandidateResultRelationSchemaV1,
		CandidateSHA256:   ExactObjectiveContextSHA(input.Candidate),
		KindReceiptSHA256: kindSHA256, CardinalityReceiptSHA256: cardinalitySHA256,
		Relation: relation,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func canonicalApplicationRequirementCandidateResultRelation(
	input ApplicationRequirementCandidateResultRelationInput,
	relation string,
) (ApplicationRequirementCandidateResultRelationResult, error) {
	derivedPresence := ApplicationRequirementCandidateResultPresent
	if relation == ApplicationRequirementNoDerivedResult {
		derivedPresence = ApplicationRequirementCandidateResultAbsent
	}
	derivedInput := ApplicationRequirementCandidateResultPresenceInput{
		Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
		Dimension: ApplicationRequirementDerivedValueDimension,
	}
	derived, err := applicationRequirementCandidateResultPresenceResult(
		derivedInput, derivedPresence,
	)
	if err != nil {
		return ApplicationRequirementCandidateResultRelationResult{}, err
	}
	if derivedPresence == ApplicationRequirementCandidateResultAbsent {
		if relation != ApplicationRequirementNoDerivedResult {
			return ApplicationRequirementCandidateResultRelationResult{}, fmt.Errorf(
				"relation %q contradicts absent derived value", relation,
			)
		}
		return ResolveApplicationRequirementCandidateResultRelation(input, derived, nil)
	}
	determiningPresence := ApplicationRequirementCandidateResultAbsent
	if relation == ApplicationRequirementExplicitResultRelation {
		determiningPresence = ApplicationRequirementCandidateResultPresent
	} else if relation != ApplicationRequirementMissingResultRelation {
		return ApplicationRequirementCandidateResultRelationResult{}, fmt.Errorf(
			"application requirement result relation %q is not registered", relation,
		)
	}
	determiningInput := ApplicationRequirementCandidateResultPresenceInput{
		Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
		Dimension:            ApplicationRequirementDeterminingRelationDimension,
		DerivedValuePresence: &derived,
	}
	determining, err := applicationRequirementCandidateResultPresenceResult(
		determiningInput, determiningPresence,
	)
	if err != nil {
		return ApplicationRequirementCandidateResultRelationResult{}, err
	}
	return ResolveApplicationRequirementCandidateResultRelation(input, derived, &determining)
}
