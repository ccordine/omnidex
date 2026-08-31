package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationRequirementCandidateOutcomeRelation WorkKind = "application_requirement_candidate_outcome_relation"

	ApplicationRequirementSameRuntimeOutcome      = "SAME_RUNTIME_OUTCOME"
	ApplicationRequirementDistinctRuntimeOutcomes = "DISTINCT_RUNTIME_OUTCOMES"

	ApplicationRequirementCandidateOutcomeRelationSchemaV1 = "omnidex.application-requirement-candidate-outcome-relation.v1"
)

type ApplicationRequirementCandidateOutcomeRelationInput struct {
	Candidate           string                                           `json:"candidate"`
	Kind                ApplicationRequirementCandidateKindResult        `json:"kind"`
	Cardinality         ApplicationRequirementCandidateCardinalityResult `json:"cardinality"`
	AcceptedRequirement string                                           `json:"accepted_requirement"`
}

type ApplicationRequirementCandidateOutcomeRelationResult struct {
	Schema                    string `json:"schema"`
	CandidateSHA256           string `json:"candidate_sha256"`
	AcceptedRequirementSHA256 string `json:"accepted_requirement_sha256"`
	Relation                  string `json:"relation"`
}

func NewApplicationRequirementCandidateOutcomeRelationJob(
	input ApplicationRequirementCandidateOutcomeRelationInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationRequirementCandidateOutcomeRelation,
		input,
	)
}

func (input ApplicationRequirementCandidateOutcomeRelationInput) validate() error {
	if err := validateApplicationIntentText(
		"application requirement outcome candidate",
		input.Candidate,
		maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"accepted application requirement outcome",
		input.AcceptedRequirement,
		maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	kindInput := ApplicationRequirementCandidateKindInput{Candidate: input.Candidate}
	if err := input.Kind.ValidateFor(kindInput); err != nil {
		return fmt.Errorf("validate outcome-relation candidate kind: %w", err)
	}
	if input.Kind.Relation != ApplicationRequirementCandidateTaskLocal {
		return fmt.Errorf(
			"application requirement outcome relation requires candidate kind %q",
			ApplicationRequirementCandidateTaskLocal,
		)
	}
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{
		Candidate: input.Candidate,
	}
	if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil {
		return fmt.Errorf("validate outcome-relation candidate cardinality: %w", err)
	}
	if input.Cardinality.Relation != ApplicationRequirementOneRuntimeOutcome {
		return fmt.Errorf(
			"application requirement outcome relation requires candidate cardinality %q",
			ApplicationRequirementOneRuntimeOutcome,
		)
	}
	return nil
}

func (input ApplicationRequirementCandidateOutcomeRelationInput) validateForModel() error {
	if err := input.validate(); err != nil {
		return err
	}
	if input.Candidate == input.AcceptedRequirement {
		return fmt.Errorf("application requirement outcome relation is mechanically exact")
	}
	return nil
}

func (result ApplicationRequirementCandidateOutcomeRelationResult) ValidateFor(
	input ApplicationRequirementCandidateOutcomeRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateOutcomeRelationSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate outcome-relation schema must be %q",
			ApplicationRequirementCandidateOutcomeRelationSchemaV1,
		)
	}
	if result.CandidateSHA256 != ExactObjectiveContextSHA(input.Candidate) {
		return fmt.Errorf("application requirement outcome-relation candidate hash does not match")
	}
	if result.AcceptedRequirementSHA256 != ExactObjectiveContextSHA(input.AcceptedRequirement) {
		return fmt.Errorf("application requirement outcome-relation accepted hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementSameRuntimeOutcome,
		ApplicationRequirementDistinctRuntimeOutcomes:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate outcome relation %q is not registered",
			result.Relation,
		)
	}
}

func BuildApplicationRequirementCandidateOutcomeRelationPrompt(
	input ApplicationRequirementCandidateOutcomeRelationInput,
) (string, error) {
	if err := input.validateForModel(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Decide whether the two exact one-outcome runtime requirements describe the same independently observable outcome or distinct independently observable outcomes.",
		"Reference relations: making a transformed value visible versus keeping it after a session restart is DISTINCT_RUNTIME_OUTCOMES; returning a response versus returning it before a fixed deadline is DISTINCT_RUNTIME_OUTCOMES; showing a response visually versus also delivering it to a nonvisual consumer is DISTINCT_RUNTIME_OUTCOMES; returning the value yielded by rule R versus returning the value that conforms to that same R is SAME_RUNTIME_OUTCOME.",
		"CURRENT CANDIDATE:\n" + input.Candidate,
		"ACCEPTED REQUIREMENT:\n" + input.AcceptedRequirement,
		"Use this decision order after reading the exact pair.",
		"1. Return DISTINCT_RUNTIME_OUTCOMES if exactly one statement adds runtime evidence through a different determining rule, external reference, scope, response, event, observation time, retention boundary, time bound, presentation or delivery channel, recipient, data format, or state. A runtime can satisfy a one-time output while failing a later observation or retention check, can provide an output while missing its time bound, and can show a value while failing to deliver it through another channel. Each is distinct even though the value is shared.",
		"2. Conformance of the identical value to its identical already-named determining rule is not added evidence. A value named as the result yielded by a rule already must conform to that rule. If the other statement only restates that relation, return SAME_RUNTIME_OUTCOME. A modifier alone does not add evidence, and producing, returning, or showing that sole value does not split it when no different delivery is required.",
		"3. Otherwise return SAME_RUNTIME_OUTCOME only for a paraphrase that adds no runtime evidence. Shared input, subject, element, or dependency alone is insufficient; all other pairs are DISTINCT_RUNTIME_OUTCOMES.",
		"Return exactly one raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"FINAL QUESTION:\nDo these statements require SAME_RUNTIME_OUTCOME or DISTINCT_RUNTIME_OUTCOMES? Return only that registered value.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateOutcomeRelationResult(
	input ApplicationRequirementCandidateOutcomeRelationInput,
	raw string,
) (ApplicationRequirementCandidateOutcomeRelationResult, error) {
	var zero ApplicationRequirementCandidateOutcomeRelationResult
	if err := input.validateForModel(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate outcome relation",
		raw,
		maximumStringBytes(
			ApplicationRequirementSameRuntimeOutcome,
			ApplicationRequirementDistinctRuntimeOutcomes,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	result, err := applicationRequirementCandidateOutcomeRelationResult(input, leaf)
	if err != nil {
		return zero, err
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func applicationRequirementCandidateOutcomeRelationResult(
	input ApplicationRequirementCandidateOutcomeRelationInput,
	relation string,
) (ApplicationRequirementCandidateOutcomeRelationResult, error) {
	return ApplicationRequirementCandidateOutcomeRelationResult{
		Schema:                    ApplicationRequirementCandidateOutcomeRelationSchemaV1,
		CandidateSHA256:           ExactObjectiveContextSHA(input.Candidate),
		AcceptedRequirementSHA256: ExactObjectiveContextSHA(input.AcceptedRequirement),
		Relation:                  relation,
	}, nil
}
