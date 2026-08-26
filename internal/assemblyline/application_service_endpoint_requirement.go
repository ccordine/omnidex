package assemblyline

import "fmt"

const ApplicationServiceEndpointRequirementSchemaV1 = "omnidex.application-service-endpoint-requirement.v1"

type ApplicationServiceEndpointRequirement string

const (
	ApplicationServiceEndpointRequired ApplicationServiceEndpointRequirement = "endpoint_required"
	ApplicationServiceSupportOnly      ApplicationServiceEndpointRequirement = "support_only"
)

type ApplicationServiceEndpointRequirementInput struct {
	ProductContext     string   `json:"product_context"`
	RequirementQuote   string   `json:"requirement_quote"`
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type ApplicationServiceEndpointRequirementResult struct {
	Schema              string                                `json:"schema"`
	EndpointRequirement ApplicationServiceEndpointRequirement `json:"endpoint_requirement"`
}

func ProjectApplicationServiceEndpointRequirementInput(
	productContext string,
	task FrozenApplicationTask,
) (ApplicationServiceEndpointRequirementInput, error) {
	input := ApplicationServiceEndpointRequirementInput{
		ProductContext: productContext, RequirementQuote: task.RequirementQuote,
		Objective:          task.Objective,
		RequiredBehaviors:  append([]string(nil), task.RequiredBehaviors...),
		AcceptanceCriteria: append([]string(nil), task.AcceptanceCriteria...),
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
	if err := validateApplicationWorkloadLine(
		"service endpoint requirement objective", input.Objective, maxApplicationObjectiveRunes,
	); err != nil {
		return err
	}
	if err := validateApplicationJobSpecificationList(
		"service endpoint requirement behavior", input.RequiredBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
	); err != nil {
		return err
	}
	return validateApplicationJobSpecificationList(
		"service endpoint requirement criterion", input.AcceptanceCriteria,
		maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
	)
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
	switch result.EndpointRequirement {
	case ApplicationServiceEndpointRequired, ApplicationServiceSupportOnly:
		return nil
	default:
		return fmt.Errorf(
			"service endpoint requirement %q is unsupported",
			result.EndpointRequirement,
		)
	}
}
