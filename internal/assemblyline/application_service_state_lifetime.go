package assemblyline

import "fmt"

const ApplicationServiceStateLifetimeSchemaV1 = "omnidex.application-service-state-lifetime.v1"

type ApplicationServiceStateLifetime string

const (
	ApplicationServiceStateRequestLocalOnly              ApplicationServiceStateLifetime = "request_local_only"
	ApplicationServiceStateCrossRequestAuthorityRequired ApplicationServiceStateLifetime = "cross_request_authority_required"
)

type ApplicationServiceStateLifetimeInput struct {
	ProductContext   string `json:"product_context"`
	RequirementQuote string `json:"requirement_quote"`
}

type ApplicationServiceStateLifetimeResult struct {
	Schema        string                          `json:"schema"`
	StateLifetime ApplicationServiceStateLifetime `json:"state_lifetime"`
}

func ProjectApplicationServiceStateLifetimeInput(
	authority ApplicationTaskRuntimeAuthority,
) (ApplicationServiceStateLifetimeInput, error) {
	input := ApplicationServiceStateLifetimeInput{
		ProductContext: authority.ProductQuote, RequirementQuote: authority.RequirementQuote,
	}
	if err := input.validate(); err != nil {
		return ApplicationServiceStateLifetimeInput{}, err
	}
	return input, nil
}

func NewApplicationServiceStateLifetimeJob(
	input ApplicationServiceStateLifetimeInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceStateLifetime, input, input.validate)
}

func (input ApplicationServiceStateLifetimeInput) validate() error {
	if err := validateApplicationProductQuote(
		"service state lifetime product", input.ProductContext,
	); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"service state lifetime requirement", input.RequirementQuote, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	return nil
}

func (result ApplicationServiceStateLifetimeResult) ValidateFor(
	input ApplicationServiceStateLifetimeInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceStateLifetimeSchemaV1 {
		return fmt.Errorf(
			"service state lifetime schema must be %q",
			ApplicationServiceStateLifetimeSchemaV1,
		)
	}
	switch result.StateLifetime {
	case ApplicationServiceStateRequestLocalOnly,
		ApplicationServiceStateCrossRequestAuthorityRequired:
		return nil
	default:
		return fmt.Errorf("service state lifetime %q is unsupported", result.StateLifetime)
	}
}
