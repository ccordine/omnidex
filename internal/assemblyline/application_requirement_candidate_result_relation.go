package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkApplicationRequirementCandidateResultRelation WorkKind = "application_requirement_candidate_result_relation"

	ApplicationRequirementNoDerivedResult        = "NO_DERIVED_RESULT"
	ApplicationRequirementExplicitResultRelation = "EXPLICIT_DERIVED_RESULT_RELATION"
	ApplicationRequirementMissingResultRelation  = "MISSING_DERIVED_RESULT_RELATION"

	ApplicationRequirementCandidateResultRelationSchemaV1 = "omnidex.application-requirement-candidate-result-relation.v1"
)

type ApplicationRequirementCandidateResultRelationInput struct {
	Candidate   string                                           `json:"candidate"`
	Kind        ApplicationRequirementCandidateKindResult        `json:"kind"`
	Cardinality ApplicationRequirementCandidateCardinalityResult `json:"cardinality"`
}

type ApplicationRequirementCandidateResultRelationResult struct {
	Schema                   string `json:"schema"`
	CandidateSHA256          string `json:"candidate_sha256"`
	KindReceiptSHA256        string `json:"kind_receipt_sha256"`
	CardinalityReceiptSHA256 string `json:"cardinality_receipt_sha256"`
	Relation                 string `json:"relation"`
}

func NewApplicationRequirementCandidateResultRelationJob(
	input ApplicationRequirementCandidateResultRelationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateResultRelation, input, input.validate,
	)
}

func (input ApplicationRequirementCandidateResultRelationInput) validate() error {
	if err := validateApplicationIntentText(
		"application requirement candidate", input.Candidate, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	kindInput := ApplicationRequirementCandidateKindInput{Candidate: input.Candidate}
	if err := input.Kind.ValidateFor(kindInput); err != nil {
		return fmt.Errorf("validate result-relation candidate kind: %w", err)
	}
	if input.Kind.Relation != ApplicationRequirementCandidateTaskLocal {
		return fmt.Errorf(
			"application requirement result-relation classification requires code-established kind %q",
			ApplicationRequirementCandidateTaskLocal,
		)
	}
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{
		Candidate: input.Candidate,
	}
	if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil {
		return fmt.Errorf("validate result-relation candidate cardinality: %w", err)
	}
	if input.Cardinality.Relation != ApplicationRequirementOneRuntimeOutcome {
		return fmt.Errorf(
			"application requirement result-relation classification requires code-established cardinality %q",
			ApplicationRequirementOneRuntimeOutcome,
		)
	}
	return nil
}

func (result ApplicationRequirementCandidateResultRelationResult) ValidateFor(
	input ApplicationRequirementCandidateResultRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if err := result.validateCandidateIdentity(input.Candidate); err != nil {
		return err
	}
	if err := result.validateReceiptAuthority(input.Kind, input.Cardinality); err != nil {
		return err
	}
	switch result.Relation {
	case ApplicationRequirementNoDerivedResult,
		ApplicationRequirementExplicitResultRelation,
		ApplicationRequirementMissingResultRelation:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate result-relation value %q is not registered",
			result.Relation,
		)
	}
}

// ValidateAcceptedFor validates the immutable result-relation receipt retained
// with one accepted requirement. A missing relation is a correction trigger and
// can never become accepted requirement authority.
func (result ApplicationRequirementCandidateResultRelationResult) ValidateAcceptedFor(
	candidate string,
) error {
	if err := validateApplicationIntentText(
		"application requirement candidate", candidate, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if err := result.validateCandidateIdentity(candidate); err != nil {
		return err
	}
	canonical := canonicalAcceptedApplicationRequirementResultRelationInput(candidate)
	if err := canonical.validate(); err != nil {
		return fmt.Errorf("construct canonical accepted result-relation authority: %w", err)
	}
	if err := result.validateReceiptAuthority(canonical.Kind, canonical.Cardinality); err != nil {
		return err
	}
	switch result.Relation {
	case ApplicationRequirementNoDerivedResult,
		ApplicationRequirementExplicitResultRelation:
		return nil
	case ApplicationRequirementMissingResultRelation:
		return fmt.Errorf(
			"application requirement candidate result relation %q cannot be retained",
			ApplicationRequirementMissingResultRelation,
		)
	default:
		return fmt.Errorf(
			"application requirement candidate result-relation value %q is not registered",
			result.Relation,
		)
	}
}

func (result ApplicationRequirementCandidateResultRelationResult) validateCandidateIdentity(
	candidate string,
) error {
	if result.Schema != ApplicationRequirementCandidateResultRelationSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate result-relation schema must be %q",
			ApplicationRequirementCandidateResultRelationSchemaV1,
		)
	}
	if result.CandidateSHA256 != ExactObjectiveContextSHA(candidate) {
		return fmt.Errorf("application requirement candidate result-relation hash does not match")
	}
	return nil
}

func (result ApplicationRequirementCandidateResultRelationResult) validateReceiptAuthority(
	kind ApplicationRequirementCandidateKindResult,
	cardinality ApplicationRequirementCandidateCardinalityResult,
) error {
	kindSHA256, err := applicationRequirementSemanticReceiptSHA256(kind)
	if err != nil {
		return fmt.Errorf("hash application requirement kind receipt: %w", err)
	}
	if result.KindReceiptSHA256 != kindSHA256 {
		return fmt.Errorf("application requirement result-relation kind receipt hash does not match")
	}
	cardinalitySHA256, err := applicationRequirementSemanticReceiptSHA256(cardinality)
	if err != nil {
		return fmt.Errorf("hash application requirement cardinality receipt: %w", err)
	}
	if result.CardinalityReceiptSHA256 != cardinalitySHA256 {
		return fmt.Errorf("application requirement result-relation cardinality receipt hash does not match")
	}
	return nil
}

func canonicalAcceptedApplicationRequirementResultRelationInput(
	candidate string,
) ApplicationRequirementCandidateResultRelationInput {
	candidateSHA256 := ExactObjectiveContextSHA(candidate)
	return ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate,
		Kind: ApplicationRequirementCandidateKindResult{
			Schema:          ApplicationRequirementCandidateKindSchemaV1,
			CandidateSHA256: candidateSHA256,
			Relation:        ApplicationRequirementCandidateTaskLocal,
		},
		Cardinality: ApplicationRequirementCandidateCardinalityResult{
			Schema:          ApplicationRequirementCandidateCardinalitySchemaV1,
			CandidateSHA256: candidateSHA256,
			Relation:        ApplicationRequirementOneRuntimeOutcome,
		},
	}
}

func applicationRequirementSemanticReceiptSHA256(receipt any) (string, error) {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	return ExactObjectiveContextSHA(string(encoded)), nil
}

func BuildApplicationRequirementCandidateResultRelationPrompt(
	input ApplicationRequirementCandidateResultRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic classification about the exact one-outcome runtime requirement below: does it require no derived result, explicitly state the semantic relation needed to determine its derived result, or require a derived result while leaving that relation unstated?",
		"A derived result is an observable value, selection, ordering, transformation, summary, decision, or other output whose correctness depends on a semantic relation to inputs or conditions. Its relation is explicit only when this exact requirement identifies the operation or rule and the observable operands, conditions, and result meaning precisely enough for an independent test to compute the expected result.",
		"A quality claim about an output does not identify its determining rule. An action that generically triggers a result does not identify the rule either.",
		"Classify only the exact candidate. Treat an otherwise unstated relation as supplied only when the candidate semantically entails exactly one determining rule and its operands, conditions, and result meaning; ambiguity means the relation is missing. Do not rewrite the candidate, infer another requirement, or use surrounding context.",
		"Return NO_DERIVED_RESULT when the outcome needs no derived-result oracle. Return EXPLICIT_DERIVED_RESULT_RELATION when the candidate itself supplies the determining relation. Return MISSING_DERIVED_RESULT_RELATION when it requires a derived result but omits any information needed to determine that result independently.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"EXACT ONE-OUTCOME REQUIREMENT CANDIDATE:\n" + input.Candidate,
		"FINAL QUESTION:\nWhich one registered result-relation classification applies? Return only NO_DERIVED_RESULT, EXPLICIT_DERIVED_RESULT_RELATION, or MISSING_DERIVED_RESULT_RELATION.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateResultRelationResult(
	input ApplicationRequirementCandidateResultRelationInput,
	raw string,
) (ApplicationRequirementCandidateResultRelationResult, error) {
	var zero ApplicationRequirementCandidateResultRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate result relation", raw, 40, false,
	)
	if err != nil {
		return zero, err
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
		Schema:                   ApplicationRequirementCandidateResultRelationSchemaV1,
		CandidateSHA256:          ExactObjectiveContextSHA(input.Candidate),
		KindReceiptSHA256:        kindSHA256,
		CardinalityReceiptSHA256: cardinalitySHA256,
		Relation:                 leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}
