package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationRequirementCandidateResultRelation WorkKind = "application_requirement_candidate_result_relation"

	ApplicationRequirementNoDerivedResult        = "NO_DERIVED_RESULT"
	ApplicationRequirementExplicitResultRelation = "EXPLICIT_DERIVED_RESULT_RELATION"
	ApplicationRequirementMissingResultRelation  = "MISSING_DERIVED_RESULT_RELATION"

	ApplicationRequirementDerivedValueDimension        ApplicationRequirementCandidateResultDimension = "derived_value"
	ApplicationRequirementDeterminingRelationDimension ApplicationRequirementCandidateResultDimension = "determining_relation"

	ApplicationRequirementCandidateResultPresent ApplicationRequirementCandidateResultPresence = "PRESENT"
	ApplicationRequirementCandidateResultAbsent  ApplicationRequirementCandidateResultPresence = "ABSENT"

	ApplicationRequirementCandidateResultPresenceSchemaV1 = "omnidex.application-requirement-candidate-result-presence.v1"
	ApplicationRequirementCandidateResultRelationSchemaV1 = "omnidex.application-requirement-candidate-result-relation.v1"
)

type ApplicationRequirementCandidateResultDimension string

type ApplicationRequirementCandidateResultPresence string

// ApplicationRequirementCandidateResultPresenceInput is one model-visible
// binary semantic question. The determining-relation question is legal only
// after a candidate-bound positive derived-value receipt.
type ApplicationRequirementCandidateResultPresenceInput struct {
	Candidate            string                                               `json:"candidate"`
	Kind                 ApplicationRequirementCandidateKindResult            `json:"kind"`
	Cardinality          ApplicationRequirementCandidateCardinalityResult     `json:"cardinality"`
	Dimension            ApplicationRequirementCandidateResultDimension       `json:"dimension"`
	DerivedValuePresence *ApplicationRequirementCandidateResultPresenceResult `json:"derived_value_presence,omitempty"`
}

type ApplicationRequirementCandidateResultPresenceResult struct {
	Schema          string                                        `json:"schema"`
	AuthoritySHA256 string                                        `json:"authority_sha256"`
	Presence        ApplicationRequirementCandidateResultPresence `json:"presence"`
}

// ApplicationRequirementCandidateResultRelationInput binds the code-owned
// final three-way receipt to one exact candidate. It is not a model input.
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

func NewApplicationRequirementCandidateResultPresenceJob(
	input ApplicationRequirementCandidateResultPresenceInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateResultRelation, input, input.validate,
	)
}

func (input ApplicationRequirementCandidateResultPresenceInput) validate() error {
	if err := validateApplicationRequirementCandidateResultAuthority(
		input.Candidate, input.Kind, input.Cardinality,
	); err != nil {
		return err
	}
	switch input.Dimension {
	case ApplicationRequirementDerivedValueDimension:
		if input.DerivedValuePresence != nil {
			return fmt.Errorf("derived-value presence question must not carry prior presence")
		}
	case ApplicationRequirementDeterminingRelationDimension:
		if input.DerivedValuePresence == nil {
			return fmt.Errorf("determining-relation presence question requires derived-value presence")
		}
		derivedInput := ApplicationRequirementCandidateResultPresenceInput{
			Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
			Dimension: ApplicationRequirementDerivedValueDimension,
		}
		if err := input.DerivedValuePresence.ValidateFor(derivedInput); err != nil {
			return fmt.Errorf("validate determining-relation derived-value authority: %w", err)
		}
		if input.DerivedValuePresence.Presence != ApplicationRequirementCandidateResultPresent {
			return fmt.Errorf("determining-relation presence question requires present derived value")
		}
	default:
		return fmt.Errorf(
			"application requirement candidate result dimension %q is not registered",
			input.Dimension,
		)
	}
	return nil
}

func (input ApplicationRequirementCandidateResultRelationInput) validate() error {
	return validateApplicationRequirementCandidateResultAuthority(
		input.Candidate, input.Kind, input.Cardinality,
	)
}

func validateApplicationRequirementCandidateResultAuthority(
	candidate string,
	kind ApplicationRequirementCandidateKindResult,
	cardinality ApplicationRequirementCandidateCardinalityResult,
) error {
	if err := validateApplicationIntentText(
		"application requirement candidate", candidate, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	kindInput := ApplicationRequirementCandidateKindInput{Candidate: candidate}
	if err := kind.ValidateFor(kindInput); err != nil {
		return fmt.Errorf("validate result-relation candidate kind: %w", err)
	}
	if kind.Relation != ApplicationRequirementCandidateTaskLocal {
		return fmt.Errorf(
			"application requirement result-relation classification requires code-established kind %q",
			ApplicationRequirementCandidateTaskLocal,
		)
	}
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{Candidate: candidate}
	if err := cardinality.ValidateFor(cardinalityInput); err != nil {
		return fmt.Errorf("validate result-relation candidate cardinality: %w", err)
	}
	if cardinality.Relation != ApplicationRequirementOneRuntimeOutcome {
		return fmt.Errorf(
			"application requirement result-relation classification requires code-established cardinality %q",
			ApplicationRequirementOneRuntimeOutcome,
		)
	}
	return nil
}

func (result ApplicationRequirementCandidateResultPresenceResult) ValidateFor(
	input ApplicationRequirementCandidateResultPresenceInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateResultPresenceSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate result presence schema must be %q",
			ApplicationRequirementCandidateResultPresenceSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCandidateResultPresenceAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement candidate result presence authority hash does not match")
	}
	switch result.Presence {
	case ApplicationRequirementCandidateResultPresent,
		ApplicationRequirementCandidateResultAbsent:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate result presence %q is not registered",
			result.Presence,
		)
	}
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

func BuildApplicationRequirementCandidateResultPresencePrompt(
	input ApplicationRequirementCandidateResultPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var question []string
	switch input.Dimension {
	case ApplicationRequirementDerivedValueDimension:
		question = []string{
			"Answer one semantic presence question about the exact candidate: does it assert a derived runtime value?",
			"A derived value is selected, ordered, transformed, read, extracted, decoded, hashed, grouped, aggregated, measured, calculated, or decided from inputs. A named result-bearing operation over its governed object is PRESENT even when phrased as an action.",
			"Return ABSENT when the candidate asserts only an action, control, state transition, event, message, artifact creation or availability, or unchanged supplied data. A trigger condition and qualitative adjective do not create a derived value.",
			"FINAL QUESTION:\nIs a derived runtime value PRESENT or ABSENT? Return only PRESENT or ABSENT.",
		}
	case ApplicationRequirementDeterminingRelationDimension:
		question = []string{
			"The exact candidate asserts a derived runtime value. Answer one semantic presence question: does it state an independently computable determining relation for that value?",
			"Return PRESENT only when the candidate names the necessary input or condition and determining rule. A named result-bearing operation uses its governed object as input and the operation as rule. Named orderings, digests, comparisons, selections, aggregations, and existing per-item grouping keys are rules; equal key values determine groups.",
			"An actor-supplied expression, formula, or operation, or an actor-performed calculation, supplies runtime rule and operands. Passively calling an unspecified output calculated, computed, evaluated, generated, selected, correct, best, useful, or appropriate supplies no rule.",
			"FINAL QUESTION:\nIs the independently computable determining relation PRESENT or ABSENT? Return only PRESENT or ABSENT.",
		}
	default:
		return "", fmt.Errorf(
			"application requirement candidate result dimension %q is not registered",
			input.Dimension,
		)
	}
	return strings.Join([]string{
		question[0], question[1], question[2],
		"Inspect only the exact candidate. Return only the raw registered presence with no JSON, label, Markdown, or explanation.",
		"EXACT REQUIREMENT CANDIDATE:\n" + input.Candidate,
		question[3],
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateResultPresenceResult(
	input ApplicationRequirementCandidateResultPresenceInput,
	raw string,
) (ApplicationRequirementCandidateResultPresenceResult, error) {
	var zero ApplicationRequirementCandidateResultPresenceResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate result presence",
		raw,
		maximumStringBytes(
			string(ApplicationRequirementCandidateResultPresent),
			string(ApplicationRequirementCandidateResultAbsent),
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationRequirementCandidateResultPresenceAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateResultPresenceResult{
		Schema:          ApplicationRequirementCandidateResultPresenceSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Presence:        ApplicationRequirementCandidateResultPresence(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

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
		Schema:                   ApplicationRequirementCandidateResultRelationSchemaV1,
		CandidateSHA256:          ExactObjectiveContextSHA(input.Candidate),
		KindReceiptSHA256:        kindSHA256,
		CardinalityReceiptSHA256: cardinalitySHA256,
		Relation:                 relation,
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
	derived, err := DecodeApplicationRequirementCandidateResultPresenceResult(
		derivedInput, string(derivedPresence),
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
	determining, err := DecodeApplicationRequirementCandidateResultPresenceResult(
		determiningInput, string(determiningPresence),
	)
	if err != nil {
		return ApplicationRequirementCandidateResultRelationResult{}, err
	}
	return ResolveApplicationRequirementCandidateResultRelation(input, derived, &determining)
}

func applicationRequirementCandidateResultPresenceAuthoritySHA256(
	input ApplicationRequirementCandidateResultPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf(
			"encode application requirement candidate result presence authority: %w", err,
		)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
