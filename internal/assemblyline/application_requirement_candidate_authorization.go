package assemblyline

import (
	"fmt"
)

const (
	WorkApplicationRequirementCandidateAuthorization WorkKind = "application_requirement_candidate_authorization"

	ApplicationRequirementCandidateEntailed    = "ENTAILED_BY_CURRENT_REQUEST"
	ApplicationRequirementCandidateNotEntailed = "NOT_ENTAILED_BY_CURRENT_REQUEST"

	ApplicationRequirementCandidateAuthorizationSchemaV1 = "omnidex.application-requirement-candidate-authorization.v1"
)

type ApplicationRequirementCandidateAuthorizationInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
	Candidate   string             `json:"candidate"`
}

type ApplicationRequirementCandidateAuthorizationResult struct {
	Schema   string `json:"schema"`
	Relation string `json:"relation"`
}

func NewApplicationRequirementCandidateAuthorizationJob(
	input ApplicationRequirementCandidateAuthorizationInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationRequirementCandidateAuthorization,
		input,
	)
}

// ResolveExactSourceApplicationRequirementCandidateAuthorization resolves the
// only authorization relation code can prove from text identity alone. An
// exact copy of the complete immutable request is necessarily entailed by that
// request. Substrings, paraphrases, and context-derived candidates remain
// unresolved and require the ordinary semantic relation.
func ResolveExactSourceApplicationRequirementCandidateAuthorization(
	input ApplicationRequirementCandidateAuthorizationInput,
) (ApplicationRequirementCandidateAuthorizationResult, bool, error) {
	var zero ApplicationRequirementCandidateAuthorizationResult
	if err := input.validate(); err != nil {
		return zero, false, err
	}
	if input.Candidate != input.UserRequest {
		return zero, false, nil
	}
	result, err := applicationRequirementCandidateAuthorizationResult(
		input, ApplicationRequirementCandidateEntailed,
	)
	if err != nil {
		return zero, false, err
	}
	return result, true, nil
}

func (input ApplicationRequirementCandidateAuthorizationInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	return validateApplicationIntentText(
		"requirement authorization candidate",
		input.Candidate,
		maxRequirementQuoteBytes,
	)
}

func BuildApplicationRequirementCandidateAuthorizationPrompt(
	input ApplicationRequirementCandidateAuthorizationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	choices, err := applicationRequirementCandidateAuthorizationOpaqueChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Is every semantic detail in the candidate required by the software request and established facts? The candidate may describe only one requested part; it need not repeat other requirements or construction instructions. Judge the meaning, not the wording. Describing requested behavior as something the finished software does adds nothing.",
		[]string{
			projection,
			"Candidate:\n" + input.Candidate,
		},
		choices,
	)
}

func DecodeApplicationRequirementCandidateAuthorizationResult(
	input ApplicationRequirementCandidateAuthorizationInput,
	raw string,
) (ApplicationRequirementCandidateAuthorizationResult, error) {
	var zero ApplicationRequirementCandidateAuthorizationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := applicationRequirementCandidateAuthorizationOpaqueChoices()
	if err != nil {
		return zero, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	return applicationRequirementCandidateAuthorizationResult(input, leaf)
}

func applicationRequirementCandidateAuthorizationResult(
	input ApplicationRequirementCandidateAuthorizationInput,
	relation string,
) (ApplicationRequirementCandidateAuthorizationResult, error) {
	var zero ApplicationRequirementCandidateAuthorizationResult
	result := ApplicationRequirementCandidateAuthorizationResult{
		Schema:   ApplicationRequirementCandidateAuthorizationSchemaV1,
		Relation: relation,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func applicationRequirementCandidateAuthorizationOpaqueChoices() ([]OpaqueModelChoice, error) {
	entailed, err := NewOpaqueModelChoice(
		"Every semantic detail in the candidate is grounded by the request and established facts.",
		ApplicationRequirementCandidateEntailed,
	)
	if err != nil {
		return nil, err
	}
	notEntailed, err := NewOpaqueModelChoice(
		"The candidate adds at least one semantic detail not grounded by the request and established facts.",
		ApplicationRequirementCandidateNotEntailed,
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{entailed, notEntailed}, nil
}

func (result ApplicationRequirementCandidateAuthorizationResult) ValidateFor(
	input ApplicationRequirementCandidateAuthorizationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateAuthorizationSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate authorization schema must be %q",
			ApplicationRequirementCandidateAuthorizationSchemaV1,
		)
	}
	switch result.Relation {
	case ApplicationRequirementCandidateEntailed,
		ApplicationRequirementCandidateNotEntailed:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate authorization value %q is not registered",
			result.Relation,
		)
	}
}
