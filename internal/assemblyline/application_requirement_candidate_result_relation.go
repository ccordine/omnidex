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
// with one accepted requirement. A missing relation requires separate request
// grounding and can never become accepted requirement authority.
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
		"Classify one fact about this exact one-outcome runtime requirement: its derived-result relation.",
		"Apply these steps in order:",
		"1. Detect a derived value when the candidate requires an observable value selected, ordered, transformed, hashed, grouped, aggregated, measured, calculated, or decided from inputs or conditions. Displaying that value does not remove the relation. Data described as converted or transformed is not supplied data unchanged.",
		"2. For a derived value, return EXPLICIT_DERIVED_RESULT_RELATION if the candidate names the necessary input or condition and the determining rule; otherwise return MISSING_DERIVED_RESULT_RELATION. A named standard transformation, ordering, digest algorithm, comparison or selection rule, aggregation, or existing per-item grouping key is a stated rule; do not demand implementation steps. For a named grouping key, equal observed key values determine groups; how the key was assigned is irrelevant. Correct, best, useful, or appropriate alone do not state a rule.",
		"3. Only when no derived value is asserted, return NO_DERIVED_RESULT for an action, control, state transition, event, message, artifact creation or availability, or display of supplied data unchanged. A qualitative adjective about such behavior does not create a derived value.",
		"Classify only the exact candidate. Do not rewrite it, infer another requirement, or use surrounding context.",
		"EXACT ONE-OUTCOME REQUIREMENT CANDIDATE:\n" + input.Candidate,
		"Return exactly one raw registered value and nothing else: NO_DERIVED_RESULT, EXPLICIT_DERIVED_RESULT_RELATION, or MISSING_DERIVED_RESULT_RELATION.",
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
