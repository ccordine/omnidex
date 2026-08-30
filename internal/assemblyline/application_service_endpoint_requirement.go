package assemblyline

import "fmt"

const ApplicationServiceEndpointRequirementSchemaV1 = "omnidex.application-service-endpoint-requirement.v1"

type ApplicationServiceEndpointRequirement string

const (
	ApplicationServiceEndpointRequired ApplicationServiceEndpointRequirement = "endpoint_required"
	ApplicationServiceSupportOnly      ApplicationServiceEndpointRequirement = "support_only"
)

type ApplicationServiceEndpointRequirementInput struct {
	ProductContext              string `json:"product_context"`
	RequirementQuote            string `json:"requirement_quote"`
	HasDirectCapabilityConsumer bool   `json:"has_direct_capability_consumer"`
}

type ApplicationServiceEndpointRequirementResult struct {
	Schema              string                                `json:"schema"`
	EndpointRequirement ApplicationServiceEndpointRequirement `json:"endpoint_requirement"`
}

func ProjectApplicationServiceEndpointRequirementInput(
	authority ApplicationTaskRuntimeAuthority,
	hasDirectCapabilityConsumer bool,
) (ApplicationServiceEndpointRequirementInput, error) {
	input := ApplicationServiceEndpointRequirementInput{
		ProductContext: authority.ProductQuote, RequirementQuote: authority.RequirementQuote,
		HasDirectCapabilityConsumer: hasDirectCapabilityConsumer,
	}
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointRequirementInput{}, err
	}
	return input, nil
}

func NewApplicationServiceEndpointRequirementJob(
	input ApplicationServiceEndpointRequirementInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceEndpointRequirement, input, input.validate)
}

func (input ApplicationServiceEndpointRequirementInput) validate() error {
	if err := validateApplicationProductQuote(
		"service endpoint requirement product", input.ProductContext,
	); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"service endpoint requirement quote", input.RequirementQuote, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if !input.HasDirectCapabilityConsumer {
		return fmt.Errorf(
			"service endpoint requirement semantic input requires one code-proven direct capability consumer",
		)
	}
	return nil
}

func (requirement ApplicationServiceEndpointRequirement) Validate() error {
	switch requirement {
	case ApplicationServiceEndpointRequired, ApplicationServiceSupportOnly:
		return nil
	default:
		return fmt.Errorf("service endpoint requirement %q is unsupported", requirement)
	}
}

func (result ApplicationServiceEndpointRequirementResult) ValidateFor(
	input ApplicationServiceEndpointRequirementInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointRequirementSchemaV1 {
		return fmt.Errorf(
			"service endpoint requirement schema must be %q",
			ApplicationServiceEndpointRequirementSchemaV1,
		)
	}
	return result.EndpointRequirement.Validate()
}
