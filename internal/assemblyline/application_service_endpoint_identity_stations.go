package assemblyline

import "strings"

func BuildApplicationServiceEndpointExposurePrompt(
	input ApplicationServiceEndpointExposureInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input.Authority, nil,
		"Determine who may reach the one HTTP endpoint required by the exact endpoint requirement.",
		"Return exactly one raw exposure value from this compatible set: "+
			strings.Join(applicationServiceEndpointExposureValues(), ", ")+".",
	)
}

func DecodeApplicationServiceEndpointExposureResult(
	input ApplicationServiceEndpointExposureInput,
	raw string,
) (ApplicationServiceEndpointExposureResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointExposureResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("service endpoint exposure", raw, 64, false)
	if err != nil {
		return ApplicationServiceEndpointExposureResult{}, err
	}
	result := ApplicationServiceEndpointExposureResult{
		Schema:   ApplicationServiceEndpointExposureSchemaV1,
		Exposure: ApplicationServiceEndpointExposure(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointExposureResult{}, err
	}
	return result, nil
}

func BuildApplicationServiceEndpointMethodPrompt(
	input ApplicationServiceEndpointMethodInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input.Authority, nil,
		"Determine the one HTTP method whose semantics match the exact accepted endpoint requirement.",
		"Return exactly one raw method value from this compatible set: "+
			strings.Join(applicationServiceEndpointMethodValues(), ", ")+".",
	)
}

func DecodeApplicationServiceEndpointMethodResult(
	input ApplicationServiceEndpointMethodInput,
	raw string,
) (ApplicationServiceEndpointMethodResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointMethodResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("service endpoint method", raw, 16, false)
	if err != nil {
		return ApplicationServiceEndpointMethodResult{}, err
	}
	result := ApplicationServiceEndpointMethodResult{
		Schema: ApplicationServiceEndpointMethodSchemaV1,
		Method: ApplicationServiceEndpointMethod(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointMethodResult{}, err
	}
	return result, nil
}

func BuildApplicationServiceEndpointRouteTemplatePrompt(
	input ApplicationServiceEndpointRouteTemplateInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input.Authority, nil,
		"Determine the one normalized HTTP route template that names the exact accepted endpoint requirement.",
		"Return exactly one raw normalized route template using lowercase literal segments or {lower_snake_case} parameter segments.",
	)
}

func DecodeApplicationServiceEndpointRouteTemplateResult(
	input ApplicationServiceEndpointRouteTemplateInput,
	raw string,
) (ApplicationServiceEndpointRouteTemplateResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointRouteTemplateResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"service endpoint route template", raw, maxApplicationServiceRouteBytes, false,
	)
	if err != nil {
		return ApplicationServiceEndpointRouteTemplateResult{}, err
	}
	result := ApplicationServiceEndpointRouteTemplateResult{
		Schema:        ApplicationServiceEndpointRouteTemplateSchemaV1,
		RouteTemplate: leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointRouteTemplateResult{}, err
	}
	return result, nil
}
