package assemblyline

import (
	"fmt"
)

const (
	WorkApplicationRequirementCandidateCardinality WorkKind = "application_requirement_candidate_cardinality"

	ApplicationRequirementOneRuntimeOutcome       = "ONE_RUNTIME_OUTCOME"
	ApplicationRequirementMultipleRuntimeOutcomes = "MULTIPLE_RUNTIME_OUTCOMES"

	ApplicationRequirementCandidateCardinalitySchemaV1 = "omnidex.application-requirement-candidate-cardinality.v1"
)

type ApplicationRequirementCandidateCardinalityInput struct {
	Candidate string `json:"candidate"`
}

type ApplicationRequirementCandidateCardinalityResult struct {
	Schema          string `json:"schema"`
	CandidateSHA256 string `json:"candidate_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationRequirementCandidateCardinalityJob(
	input ApplicationRequirementCandidateCardinalityInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationRequirementCandidateCardinality, input,
	)
}

func (input ApplicationRequirementCandidateCardinalityInput) validate() error {
	return validateApplicationIntentText(
		"application requirement candidate", input.Candidate, maxRequirementQuoteBytes,
	)
}

func (result ApplicationRequirementCandidateCardinalityResult) ValidateFor(
	input ApplicationRequirementCandidateCardinalityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateCardinalitySchemaV1 {
		return fmt.Errorf(
			"application requirement candidate cardinality schema must be %q",
			ApplicationRequirementCandidateCardinalitySchemaV1,
		)
	}
	if result.CandidateSHA256 != ExactObjectiveContextSHA(input.Candidate) {
		return fmt.Errorf("application requirement candidate cardinality hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementOneRuntimeOutcome,
		ApplicationRequirementMultipleRuntimeOutcomes:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate cardinality value %q is not registered",
			result.Relation,
		)
	}
}

func BuildApplicationRequirementCandidateCardinalityPrompt(
	input ApplicationRequirementCandidateCardinalityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationRequirementCandidateCardinalityOpaqueChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"How many independently testable runtime outcomes does this candidate describe? A runtime outcome is one concrete capability, observable behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output format. Count distinct concrete actions, elements, qualities, or outcomes separately. Operands, a condition or trigger, a determining relation, and the resulting output that jointly define one end-to-end behavior remain one outcome. A second observable response meaning is separate. Grammatical subjects, container wording, modifiers, and purpose clauses do not add outcomes by themselves.",
		[]string{"Requirement candidate:\n" + input.Candidate},
		choices,
	)
}

func DecodeApplicationRequirementCandidateCardinalityResult(
	input ApplicationRequirementCandidateCardinalityInput,
	raw string,
) (ApplicationRequirementCandidateCardinalityResult, error) {
	var zero ApplicationRequirementCandidateCardinalityResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := applicationRequirementCandidateCardinalityOpaqueChoices()
	if err != nil {
		return zero, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateCardinalityResult{
		Schema:          ApplicationRequirementCandidateCardinalitySchemaV1,
		CandidateSHA256: ExactObjectiveContextSHA(input.Candidate),
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func applicationRequirementCandidateCardinalityOpaqueChoices() ([]OpaqueModelChoice, error) {
	one, err := NewOpaqueModelChoice(
		"One independent runtime assertion is sufficient to test the complete candidate.",
		ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		return nil, err
	}
	multiple, err := NewOpaqueModelChoice(
		"Two or more independent runtime assertions are required to test the complete candidate.",
		ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{one, multiple}, nil
}
