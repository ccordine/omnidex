package assemblyline

import (
	"fmt"
	"strings"
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
	return strings.Join([]string{
		"Answer one semantic relation about the exact candidate below: does it describe exactly one independently testable runtime outcome, or multiple independently testable runtime outcomes?",
		"A runtime outcome is one concrete capability, observable behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output format. Count distinct concrete actions, elements, qualities, or outcomes separately even when one sentence or umbrella phrase joins them.",
		"The observable operands, condition or trigger, determining relation, and resulting output that jointly define one end-to-end behavior are parts of that one outcome, not separate outcomes.",
		"That end-to-end grouping applies to one required observable response meaning. A second required response meaning is a separate outcome whether its condition or trigger differs or is shared.",
		"Do not count the grammatical subject, presentation or container wording, modifiers, or a purpose clause as additional outcomes. Presenting one concrete behavior or element inside an interface, page, command, output, or other container is one outcome unless the candidate independently requires another concrete behavior or element.",
		"Return ONE_RUNTIME_OUTCOME only when one independent runtime assertion is sufficient to test the whole candidate. Return MULTIPLE_RUNTIME_OUTCOMES when two or more independent runtime assertions are required.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"EXACT REQUIREMENT CANDIDATE:\n" + input.Candidate,
		"FINAL QUESTION:\nDoes this exact candidate contain one runtime outcome or multiple runtime outcomes? Return only ONE_RUNTIME_OUTCOME or MULTIPLE_RUNTIME_OUTCOMES.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateCardinalityResult(
	input ApplicationRequirementCandidateCardinalityInput,
	raw string,
) (ApplicationRequirementCandidateCardinalityResult, error) {
	var zero ApplicationRequirementCandidateCardinalityResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate cardinality", raw, 32, false,
	)
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
