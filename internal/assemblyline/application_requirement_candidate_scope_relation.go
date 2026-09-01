package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
)

const (
	WorkApplicationRequirementCandidateScopeRelation WorkKind = "application_requirement_candidate_scope_relation"

	ApplicationRequirementCandidateScopeReasonableDerivation = "REASONABLE_DERIVATION"
	ApplicationRequirementCandidateScopeSpeculativeReview    = "SPECULATIVE_REVIEW"
	ApplicationRequirementCandidateScopeConcreteConflict     = "CONCRETE_SCOPE_CONFLICT"

	ApplicationRequirementCandidateScopeRelationSchemaV1 = "omnidex.application-requirement-candidate-scope-relation.v1"
)

// ApplicationRequirementCandidateScopeRelationInput binds one scope annotation
// to a candidate already proven not to be completely request-entailed.
type ApplicationRequirementCandidateScopeRelationInput struct {
	UserRequest   string                                             `json:"user_request"`
	Context       ApplicationContext                                 `json:"context"`
	Candidate     string                                             `json:"candidate"`
	Authorization ApplicationRequirementCandidateAuthorizationResult `json:"authorization"`
	ScopeMode     model.CodingScopeMode                              `json:"scope_mode"`
}

// ApplicationRequirementCandidateScopeRelationResult is annotation evidence.
// It does not authorize acceptance, rejection, or any workflow transition.
type ApplicationRequirementCandidateScopeRelationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationRequirementCandidateScopeRelationJob(
	input ApplicationRequirementCandidateScopeRelationInput,
) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationRequirementCandidateScopeRelation, input)
}

func (input ApplicationRequirementCandidateScopeRelationInput) validate() error {
	if err := input.ScopeMode.Validate(); err != nil {
		return fmt.Errorf("application requirement candidate scope relation: %w", err)
	}
	authorizationInput := ApplicationRequirementCandidateAuthorizationInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
		Candidate:   input.Candidate,
	}
	if err := input.Authorization.ValidateFor(authorizationInput); err != nil {
		return fmt.Errorf("application requirement candidate scope relation authorization: %w", err)
	}
	if input.Authorization.Relation != ApplicationRequirementCandidateNotEntailed {
		return fmt.Errorf(
			"application requirement candidate scope relation requires a bound not-entailed authorization",
		)
	}
	return nil
}

func BuildApplicationRequirementCandidateScopeRelationPrompt(
	input ApplicationRequirementCandidateScopeRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationRequirementCandidateScopeRelationOpaqueChoices()
	if err != nil {
		return "", err
	}
	threshold, err := applicationRequirementCandidateScopeRelationThreshold(input.ScopeMode)
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Which description best characterizes the candidate's scope relationship to the software request and established facts?",
		[]string{
			renderApplicationContextModelProjection(input.UserRequest, input.Context),
			"Candidate:\n" + input.Candidate,
			"The candidate contains at least one detail not directly grounded by the request and facts. Classify only the nature of that additional scope.",
			threshold,
		},
		choices,
	)
}

func applicationRequirementCandidateScopeRelationThreshold(
	mode model.CodingScopeMode,
) (string, error) {
	switch mode {
	case model.CodingScopeModeStrict:
		return "Apply a strict classification threshold: additional meaning is a reasonable derivation only when it is necessary to fulfill the request or required by an established repository fact. Merely useful, customary, or optional aligned additions require speculative review.", nil
	case model.CodingScopeModeNormal:
		return "Apply a normal classification threshold: additional meaning is a reasonable derivation when it is a necessary or ordinary useful consequence reasonably justified by the objective or established facts. Cohesive but merely optional enhancements require speculative review.", nil
	case model.CodingScopeModeExpansive:
		return "Apply an expansive classification threshold: additional meaning is a reasonable derivation when it is a cohesive, useful, objective-aligned possibility that does not conflict with or materially depart from the request or established facts. Use speculative review only when that alignment or usefulness genuinely requires user judgment.", nil
	default:
		return "", fmt.Errorf(
			"application requirement candidate scope relation mode %q is unsupported",
			mode,
		)
	}
}

func DecodeApplicationRequirementCandidateScopeRelationResult(
	input ApplicationRequirementCandidateScopeRelationInput,
	raw string,
) (ApplicationRequirementCandidateScopeRelationResult, error) {
	var zero ApplicationRequirementCandidateScopeRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := applicationRequirementCandidateScopeRelationOpaqueChoices()
	if err != nil {
		return zero, err
	}
	relation, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationRequirementCandidateScopeRelationAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateScopeRelationResult{
		Schema:          ApplicationRequirementCandidateScopeRelationSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        relation,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func applicationRequirementCandidateScopeRelationOpaqueChoices() ([]OpaqueModelChoice, error) {
	reasonable, err := NewOpaqueModelChoice(
		"The candidate's additional meaning is an ordinary objective or repository-justified useful consequence of the request and established facts, with no material departure from them.",
		ApplicationRequirementCandidateScopeReasonableDerivation,
	)
	if err != nil {
		return nil, err
	}
	speculative, err := NewOpaqueModelChoice(
		"The candidate is cohesive and aligned possible work, but it is optional or requires user judgment before becoming part of the objective.",
		ApplicationRequirementCandidateScopeSpeculativeReview,
	)
	if err != nil {
		return nil, err
	}
	conflict, err := NewOpaqueModelChoice(
		"The candidate conflicts with an explicit prohibition or established fact, depends on an impossible established assumption, concerns an unrelated or wrong project, or materially departs from the request.",
		ApplicationRequirementCandidateScopeConcreteConflict,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{reasonable, speculative, conflict}, nil
}

func (result ApplicationRequirementCandidateScopeRelationResult) ValidateFor(
	input ApplicationRequirementCandidateScopeRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateScopeRelationSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate scope relation schema must be %q",
			ApplicationRequirementCandidateScopeRelationSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCandidateScopeRelationAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement candidate scope relation authority hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementCandidateScopeReasonableDerivation,
		ApplicationRequirementCandidateScopeSpeculativeReview,
		ApplicationRequirementCandidateScopeConcreteConflict:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate scope relation value %q is not registered",
			result.Relation,
		)
	}
}

func applicationRequirementCandidateScopeRelationAuthoritySHA256(
	input ApplicationRequirementCandidateScopeRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement candidate scope relation authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
