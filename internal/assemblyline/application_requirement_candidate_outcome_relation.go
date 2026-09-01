package assemblyline

import (
	"fmt"
)

const (
	WorkApplicationRequirementCandidateOutcomeRelation WorkKind = "application_requirement_candidate_outcome_relation"

	ApplicationRequirementSameRuntimeOutcome      = "SAME_RUNTIME_OUTCOME"
	ApplicationRequirementDistinctRuntimeOutcomes = "DISTINCT_RUNTIME_OUTCOMES"

	ApplicationRequirementCandidateOutcomeRelationSchemaV1 = "omnidex.application-requirement-candidate-outcome-relation.v1"
)

type ApplicationRequirementCandidateOutcomeRelationInput struct {
	Candidate              string                                              `json:"candidate"`
	Kind                   ApplicationRequirementCandidateKindResult           `json:"kind"`
	Cardinality            ApplicationRequirementCandidateCardinalityResult    `json:"cardinality"`
	AcceptedRequirement    string                                              `json:"accepted_requirement"`
	AcceptedResultRelation ApplicationRequirementCandidateResultRelationResult `json:"accepted_result_relation"`
}

type ApplicationRequirementCandidateOutcomeRelationResult struct {
	Schema                    string `json:"schema"`
	CandidateSHA256           string `json:"candidate_sha256"`
	AcceptedRequirementSHA256 string `json:"accepted_requirement_sha256"`
	KindReceiptSHA256         string `json:"kind_receipt_sha256"`
	CardinalityReceiptSHA256  string `json:"cardinality_receipt_sha256"`
	AcceptedReceiptSHA256     string `json:"accepted_receipt_sha256"`
	Relation                  string `json:"relation"`
}

func NewApplicationRequirementCandidateOutcomeRelationJob(
	input ApplicationRequirementCandidateOutcomeRelationInput,
) (PortableJob, error) {
	if err := input.validateForModel(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationRequirementCandidateOutcomeRelation, input)
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
	if err := input.AcceptedResultRelation.ValidateAcceptedFor(
		input.AcceptedRequirement,
	); err != nil {
		return fmt.Errorf("validate accepted outcome-relation authority: %w", err)
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
	kindSHA256, err := applicationRequirementSemanticReceiptSHA256(input.Kind)
	if err != nil {
		return fmt.Errorf("hash outcome-relation kind receipt: %w", err)
	}
	if result.KindReceiptSHA256 != kindSHA256 {
		return fmt.Errorf("application requirement outcome-relation kind receipt hash does not match")
	}
	cardinalitySHA256, err := applicationRequirementSemanticReceiptSHA256(input.Cardinality)
	if err != nil {
		return fmt.Errorf("hash outcome-relation cardinality receipt: %w", err)
	}
	if result.CardinalityReceiptSHA256 != cardinalitySHA256 {
		return fmt.Errorf("application requirement outcome-relation cardinality receipt hash does not match")
	}
	acceptedSHA256, err := applicationRequirementSemanticReceiptSHA256(input.AcceptedResultRelation)
	if err != nil {
		return fmt.Errorf("hash accepted outcome-relation receipt: %w", err)
	}
	if result.AcceptedReceiptSHA256 != acceptedSHA256 {
		return fmt.Errorf("application requirement outcome-relation accepted receipt hash does not match")
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
	choices, err := applicationRequirementCandidateOutcomeRelationOpaqueChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Do these one-outcome runtime requirements describe the same independently observable outcome or distinct outcomes? They are distinct when exactly one adds runtime evidence through a different determining rule, external reference, scope, response, event, observation time, retention boundary, time bound, presentation or delivery channel, recipient, data format, or state. Making a value visible and retaining it after restart are distinct; an ordinary response and the same response under a fixed deadline are distinct; visual presentation and delivery to a nonvisual consumer are distinct. Conformance of an identical value to its identical already-named determining rule adds no evidence. A modifier alone adds no evidence, and producing, returning, or showing the sole value does not split it when no different delivery is required. Otherwise, only a paraphrase adding no runtime evidence describes the same outcome; shared input, subject, element, or dependency alone is insufficient.",
		[]string{
			"Current candidate:\n" + input.Candidate,
			"Accepted requirement:\n" + input.AcceptedRequirement,
		},
		choices,
	)
}

func DecodeApplicationRequirementCandidateOutcomeRelationResult(
	input ApplicationRequirementCandidateOutcomeRelationInput,
	raw string,
) (ApplicationRequirementCandidateOutcomeRelationResult, error) {
	var zero ApplicationRequirementCandidateOutcomeRelationResult
	if err := input.validateForModel(); err != nil {
		return zero, err
	}
	choices, err := applicationRequirementCandidateOutcomeRelationOpaqueChoices()
	if err != nil {
		return zero, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
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

func applicationRequirementCandidateOutcomeRelationOpaqueChoices() ([]OpaqueModelChoice, error) {
	same, err := NewOpaqueModelChoice(
		"The statements are paraphrases requiring the same independently observable runtime evidence.",
		ApplicationRequirementSameRuntimeOutcome,
	)
	if err != nil {
		return nil, err
	}
	distinct, err := NewOpaqueModelChoice(
		"At least one statement requires independently observable runtime evidence that the other does not.",
		ApplicationRequirementDistinctRuntimeOutcomes,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{same, distinct}, nil
}

func applicationRequirementCandidateOutcomeRelationResult(
	input ApplicationRequirementCandidateOutcomeRelationInput,
	relation string,
) (ApplicationRequirementCandidateOutcomeRelationResult, error) {
	var zero ApplicationRequirementCandidateOutcomeRelationResult
	kindSHA256, err := applicationRequirementSemanticReceiptSHA256(input.Kind)
	if err != nil {
		return zero, fmt.Errorf("hash outcome-relation kind receipt: %w", err)
	}
	cardinalitySHA256, err := applicationRequirementSemanticReceiptSHA256(input.Cardinality)
	if err != nil {
		return zero, fmt.Errorf("hash outcome-relation cardinality receipt: %w", err)
	}
	acceptedSHA256, err := applicationRequirementSemanticReceiptSHA256(input.AcceptedResultRelation)
	if err != nil {
		return zero, fmt.Errorf("hash accepted outcome-relation receipt: %w", err)
	}
	return ApplicationRequirementCandidateOutcomeRelationResult{
		Schema:                    ApplicationRequirementCandidateOutcomeRelationSchemaV1,
		CandidateSHA256:           ExactObjectiveContextSHA(input.Candidate),
		AcceptedRequirementSHA256: ExactObjectiveContextSHA(input.AcceptedRequirement),
		KindReceiptSHA256:         kindSHA256,
		CardinalityReceiptSHA256:  cardinalitySHA256,
		AcceptedReceiptSHA256:     acceptedSHA256,
		Relation:                  relation,
	}, nil
}
