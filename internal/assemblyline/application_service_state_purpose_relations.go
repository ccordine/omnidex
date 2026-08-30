package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

type ApplicationServiceStatePurposeScope string

const (
	ApplicationServiceStateRootPurposeScope   ApplicationServiceStatePurposeScope = "root"
	ApplicationServiceStateRecordPurposeScope ApplicationServiceStatePurposeScope = "record"

	ApplicationServiceStatePurposeNecessary    = "NECESSARY_FOR_BEHAVIOR"
	ApplicationServiceStatePurposeNotNecessary = "NOT_NECESSARY_FOR_BEHAVIOR"
	ApplicationServiceStateSamePurpose         = "SAME_STATE_PURPOSE"
	ApplicationServiceStateDistinctPurposes    = "DISTINCT_STATE_PURPOSES"

	ApplicationServiceStatePurposeNecessitySchemaV1 = "omnidex.application-service-state-purpose-necessity.v1"
	ApplicationServiceStatePurposeRelationSchemaV1  = "omnidex.application-service-state-purpose-relation.v1"
)

type ApplicationServiceStatePurposeNecessityInput struct {
	Scope            ApplicationServiceStatePurposeScope   `json:"scope"`
	Authority        ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose    string                                `json:"parent_purpose,omitempty"`
	CandidatePurpose string                                `json:"candidate_purpose"`
}

type ApplicationServiceStatePurposeNecessityResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

type ApplicationServiceStatePurposeRelationInput struct {
	Scope            ApplicationServiceStatePurposeScope `json:"scope"`
	CandidatePurpose string                              `json:"candidate_purpose"`
	AcceptedPurpose  string                              `json:"accepted_purpose"`
}

type ApplicationServiceStatePurposeRelationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationServiceStatePurposeNecessityJob(
	input ApplicationServiceStatePurposeNecessityInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationServiceStatePurposeNecessity,
		input,
		input.validate,
	)
}

func NewApplicationServiceStatePurposeRelationJob(
	input ApplicationServiceStatePurposeRelationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationServiceStatePurposeRelation,
		input,
		input.validate,
	)
}

func (input ApplicationServiceStatePurposeNecessityInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurposeScope(input.Scope); err != nil {
		return err
	}
	switch input.Scope {
	case ApplicationServiceStateRootPurposeScope:
		if input.ParentPurpose != "" {
			return fmt.Errorf("root state purpose necessity must not include a parent purpose")
		}
	case ApplicationServiceStateRecordPurposeScope:
		if err := validateApplicationServiceStatePurpose("record parent", input.ParentPurpose); err != nil {
			return err
		}
	}
	return validateApplicationServiceStatePurpose("necessity candidate", input.CandidatePurpose)
}

func (input ApplicationServiceStatePurposeRelationInput) validate() error {
	if err := validateApplicationServiceStatePurposeScope(input.Scope); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("relation candidate", input.CandidatePurpose); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("relation accepted", input.AcceptedPurpose); err != nil {
		return err
	}
	if input.CandidatePurpose == input.AcceptedPurpose {
		return fmt.Errorf("application service state purpose relation requires byte-different purposes")
	}
	return nil
}

func validateApplicationServiceStatePurposeScope(scope ApplicationServiceStatePurposeScope) error {
	switch scope {
	case ApplicationServiceStateRootPurposeScope, ApplicationServiceStateRecordPurposeScope:
		return nil
	default:
		return fmt.Errorf("application service state purpose scope %q is not registered", scope)
	}
}

func BuildApplicationServiceStatePurposeNecessityPrompt(
	input ApplicationServiceStatePurposeNecessityInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic relation question: is the exact candidate purpose necessary within the minimally sufficient durable state interface required by the directly related accepted behavior authority?",
		"Return NECESSARY_FOR_BEHAVIOR only when that authority requires the exact candidate responsibility. Return NOT_NECESSARY_FOR_BEHAVIOR for an optional, customary, speculative, presentation-only, or merely useful responsibility. Return only NECESSARY_FOR_BEHAVIOR or NOT_NECESSARY_FOR_BEHAVIOR, with no JSON, label, Markdown, or explanation.",
		"APPLICATION_SERVICE_STATE_PURPOSE_NECESSITY_AUTHORITY",
		input,
	)
}

func BuildApplicationServiceStatePurposeRelationPrompt(
	input ApplicationServiceStatePurposeRelationInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic relation question: do the exact candidate purpose and exact accepted purpose express the same durable state responsibility or distinct responsibilities?",
		"Return SAME_STATE_PURPOSE when they express the same responsibility even with different wording. Return DISTINCT_STATE_PURPOSES when each preserves a different necessary responsibility. Return only SAME_STATE_PURPOSE or DISTINCT_STATE_PURPOSES, with no JSON, label, Markdown, or explanation.",
		"APPLICATION_SERVICE_STATE_PURPOSE_RELATION_INPUT",
		input,
	)
}

func DecodeApplicationServiceStatePurposeNecessityResult(
	input ApplicationServiceStatePurposeNecessityInput,
	raw string,
) (ApplicationServiceStatePurposeNecessityResult, error) {
	var zero ApplicationServiceStatePurposeNecessityResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application service state purpose necessity",
		raw,
		maximumStringBytes(
			ApplicationServiceStatePurposeNecessary,
			ApplicationServiceStatePurposeNotNecessary,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationServiceStateSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return zero, err
	}
	result := ApplicationServiceStatePurposeNecessityResult{
		Schema:          ApplicationServiceStatePurposeNecessitySchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result ApplicationServiceStatePurposeNecessityResult) ValidateFor(
	input ApplicationServiceStatePurposeNecessityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceStatePurposeNecessitySchemaV1 {
		return fmt.Errorf("application service state purpose necessity schema must be %q", ApplicationServiceStatePurposeNecessitySchemaV1)
	}
	authoritySHA256, err := applicationServiceStateSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application service state purpose necessity authority hash does not match")
	}
	switch result.Relation {
	case ApplicationServiceStatePurposeNecessary, ApplicationServiceStatePurposeNotNecessary:
		return nil
	default:
		return fmt.Errorf("application service state purpose necessity value %q is not registered", result.Relation)
	}
}

func DecodeApplicationServiceStatePurposeRelationResult(
	input ApplicationServiceStatePurposeRelationInput,
	raw string,
) (ApplicationServiceStatePurposeRelationResult, error) {
	var zero ApplicationServiceStatePurposeRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application service state purpose relation",
		raw,
		maximumStringBytes(
			ApplicationServiceStateSamePurpose,
			ApplicationServiceStateDistinctPurposes,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationServiceStateSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return zero, err
	}
	result := ApplicationServiceStatePurposeRelationResult{
		Schema:          ApplicationServiceStatePurposeRelationSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result ApplicationServiceStatePurposeRelationResult) ValidateFor(
	input ApplicationServiceStatePurposeRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceStatePurposeRelationSchemaV1 {
		return fmt.Errorf("application service state purpose relation schema must be %q", ApplicationServiceStatePurposeRelationSchemaV1)
	}
	authoritySHA256, err := applicationServiceStateSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application service state purpose relation authority hash does not match")
	}
	switch result.Relation {
	case ApplicationServiceStateSamePurpose, ApplicationServiceStateDistinctPurposes:
		return nil
	default:
		return fmt.Errorf("application service state purpose relation value %q is not registered", result.Relation)
	}
}

func applicationServiceStateSemanticAuthoritySHA256(
	input any,
	validate func() error,
) (string, error) {
	if err := validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application service state semantic authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
