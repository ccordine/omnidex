package assemblyline

func BuildApplicationServiceEndpointExposurePrompt(
	input ApplicationServiceEndpointExposureInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine who may reach the one HTTP endpoint that exposes this accepted local task.",
		"The complete response contains the registered schema and exactly one exposure: public, authenticated, or internal.",
	)
}

func ApplicationServiceEndpointExposureResponseSchema() map[string]any {
	return serviceEndpointLeafSchema(
		ApplicationServiceEndpointExposureSchemaV1, "exposure",
		map[string]any{"type": "string", "enum": []string{
			string(ApplicationServiceEndpointPublic),
			string(ApplicationServiceEndpointAuthenticated),
			string(ApplicationServiceEndpointInternal),
		}},
	)
}

func DecodeApplicationServiceEndpointExposureResult(
	input ApplicationServiceEndpointExposureInput,
	raw string,
) (ApplicationServiceEndpointExposureResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointExposureResult{}, err
	}
	var result ApplicationServiceEndpointExposureResult
	return decodeApplicationServiceEndpointLeaf(raw, &result, func(value ApplicationServiceEndpointExposureResult) error {
		return value.ValidateFor(input)
	})
}

func BuildApplicationServiceEndpointMethodPrompt(
	input ApplicationServiceEndpointMethodInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one HTTP method whose semantics match this accepted local task.",
		"The complete response contains the registered schema and exactly one method: GET, POST, PUT, PATCH, or DELETE.",
	)
}

func ApplicationServiceEndpointMethodResponseSchema() map[string]any {
	return serviceEndpointLeafSchema(
		ApplicationServiceEndpointMethodSchemaV1, "method",
		map[string]any{"type": "string", "enum": []string{
			string(ApplicationServiceEndpointGET), string(ApplicationServiceEndpointPOST),
			string(ApplicationServiceEndpointPUT), string(ApplicationServiceEndpointPATCH),
			string(ApplicationServiceEndpointDELETE),
		}},
	)
}

func DecodeApplicationServiceEndpointMethodResult(
	input ApplicationServiceEndpointMethodInput,
	raw string,
) (ApplicationServiceEndpointMethodResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointMethodResult{}, err
	}
	var result ApplicationServiceEndpointMethodResult
	return decodeApplicationServiceEndpointLeaf(raw, &result, func(value ApplicationServiceEndpointMethodResult) error {
		return value.ValidateFor(input)
	})
}

func BuildApplicationServiceEndpointRouteTemplatePrompt(
	input ApplicationServiceEndpointRouteTemplateInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one normalized HTTP route template that names this accepted local task.",
		"The complete response contains the registered schema and exactly one normalized route_template using lowercase literal segments or {lower_snake_case} parameter segments.",
	)
}

func ApplicationServiceEndpointRouteTemplateResponseSchema() map[string]any {
	return serviceEndpointLeafSchema(
		ApplicationServiceEndpointRouteTemplateSchemaV1, "route_template",
		map[string]any{"type": "string", "minLength": 1, "maxLength": maxApplicationServiceRouteBytes},
	)
}

func DecodeApplicationServiceEndpointRouteTemplateResult(
	input ApplicationServiceEndpointRouteTemplateInput,
	raw string,
) (ApplicationServiceEndpointRouteTemplateResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointRouteTemplateResult{}, err
	}
	var result ApplicationServiceEndpointRouteTemplateResult
	return decodeApplicationServiceEndpointLeaf(raw, &result, func(value ApplicationServiceEndpointRouteTemplateResult) error {
		return value.ValidateFor(input)
	})
}
