package assemblyline

import (
	"fmt"
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

// ApplicationRequirementCandidateResultPresenceInput is code-owned authority
// for one binary semantic question. Its renderer exposes only the candidate
// and the exact question semantics. Code retains the classification and only
// asks about a determining relation after a positive derived-value result.
type ApplicationRequirementCandidateResultPresenceInput struct {
	Candidate            string                                               `json:"candidate"`
	Kind                 ApplicationRequirementCandidateKindResult            `json:"kind"`
	Cardinality          ApplicationRequirementCandidateCardinalityResult     `json:"cardinality"`
	Dimension            ApplicationRequirementCandidateResultDimension       `json:"dimension"`
	DerivedValuePresence *ApplicationRequirementCandidateResultPresenceResult `json:"derived_value_presence,omitempty"`
}

type ApplicationRequirementCandidateResultPresenceResult struct {
	Schema   string                                        `json:"schema"`
	Presence ApplicationRequirementCandidateResultPresence `json:"presence"`
}

// ApplicationRequirementCandidateResultRelationInput binds the code-owned
// final three-way receipt to one exact candidate. It is not a model input.
type ApplicationRequirementCandidateResultRelationInput struct {
	Candidate   string                                           `json:"candidate"`
	Kind        ApplicationRequirementCandidateKindResult        `json:"kind"`
	Cardinality ApplicationRequirementCandidateCardinalityResult `json:"cardinality"`
}

type ApplicationRequirementCandidateResultRelationResult struct {
	Schema   string `json:"schema"`
	Relation string `json:"relation"`
}

func NewApplicationRequirementCandidateResultPresenceJob(
	input ApplicationRequirementCandidateResultPresenceInput,
) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationRequirementCandidateResultRelation, input)
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
		return fmt.Errorf("application requirement candidate result dimension %q is not registered", input.Dimension)
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
	if err := kind.ValidateFor(ApplicationRequirementCandidateKindInput{Candidate: candidate}); err != nil {
		return fmt.Errorf("validate result-relation candidate kind: %w", err)
	}
	if kind.Relation != ApplicationRequirementCandidateTaskLocal {
		return fmt.Errorf(
			"application requirement result-relation classification requires code-established kind %q",
			ApplicationRequirementCandidateTaskLocal,
		)
	}
	if err := cardinality.ValidateFor(
		ApplicationRequirementCandidateCardinalityInput{Candidate: candidate},
	); err != nil {
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
	switch result.Presence {
	case ApplicationRequirementCandidateResultPresent, ApplicationRequirementCandidateResultAbsent:
		return nil
	default:
		return fmt.Errorf("application requirement candidate result presence %q is not registered", result.Presence)
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
	switch result.Relation {
	case ApplicationRequirementNoDerivedResult,
		ApplicationRequirementExplicitResultRelation,
		ApplicationRequirementMissingResultRelation:
		return nil
	default:
		return fmt.Errorf("application requirement candidate result-relation value %q is not registered", result.Relation)
	}
}

// ValidateAcceptedFor checks the accepted semantic value and candidate shape.
// A missing relation is never retainable.
func (result ApplicationRequirementCandidateResultRelationResult) ValidateAcceptedFor(candidate string) error {
	if err := validateApplicationIntentText(
		"application requirement candidate", candidate, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if err := result.validateCandidateIdentity(candidate); err != nil {
		return err
	}
	switch result.Relation {
	case ApplicationRequirementNoDerivedResult, ApplicationRequirementExplicitResultRelation:
		return nil
	case ApplicationRequirementMissingResultRelation:
		return fmt.Errorf("application requirement candidate result relation %q cannot be retained", result.Relation)
	default:
		return fmt.Errorf("application requirement candidate result-relation value %q is not registered", result.Relation)
	}
}

func (result ApplicationRequirementCandidateResultRelationResult) validateCandidateIdentity(candidate string) error {
	if result.Schema != ApplicationRequirementCandidateResultRelationSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate result-relation schema must be %q",
			ApplicationRequirementCandidateResultRelationSchemaV1,
		)
	}
	return nil
}
