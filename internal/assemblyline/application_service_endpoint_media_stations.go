package assemblyline

func BuildApplicationServiceEndpointRequestMediaPrompt(
	input ApplicationServiceEndpointRequestMediaInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one request media type required by this accepted local task and accepted HTTP method.",
		"The complete response contains the registered schema and exactly one request_media from the registered values.",
	)
}

func ApplicationServiceEndpointRequestMediaResponseSchema(
	input ApplicationServiceEndpointRequestMediaInput,
) (map[string]any, error) {
	candidates, err := ApplicationServiceEndpointRequestMediaCandidates(input)
	if err != nil {
		return nil, err
	}
	values := make([]string, len(candidates))
	for index, candidate := range candidates {
		values[index] = string(candidate)
	}
	return serviceEndpointLeafSchema(
		ApplicationServiceEndpointRequestMediaSchemaV1, "request_media",
		map[string]any{"type": "string", "enum": values},
	), nil
}

func DecodeApplicationServiceEndpointRequestMediaResult(
	input ApplicationServiceEndpointRequestMediaInput,
	raw string,
) (ApplicationServiceEndpointRequestMediaResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointRequestMediaResult{}, err
	}
	var result ApplicationServiceEndpointRequestMediaResult
	return decodeApplicationServiceEndpointLeaf(raw, &result, func(value ApplicationServiceEndpointRequestMediaResult) error {
		return value.ValidateFor(input)
	})
}

func BuildApplicationServiceEndpointResponseMediaPrompt(
	input ApplicationServiceEndpointResponseMediaInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one response media type produced by this accepted local task.",
		"The complete response contains the registered schema and exactly one response_media from the registered values.",
	)
}

func ApplicationServiceEndpointResponseMediaResponseSchema() map[string]any {
	return serviceEndpointLeafSchema(
		ApplicationServiceEndpointResponseMediaSchemaV1, "response_media",
		map[string]any{"type": "string", "enum": applicationServiceResponseMediaValues()},
	)
}

func DecodeApplicationServiceEndpointResponseMediaResult(
	input ApplicationServiceEndpointResponseMediaInput,
	raw string,
) (ApplicationServiceEndpointResponseMediaResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointResponseMediaResult{}, err
	}
	var result ApplicationServiceEndpointResponseMediaResult
	return decodeApplicationServiceEndpointLeaf(raw, &result, func(value ApplicationServiceEndpointResponseMediaResult) error {
		return value.ValidateFor(input)
	})
}

func BuildApplicationServiceEndpointSuccessStatusPrompt(
	input ApplicationServiceEndpointSuccessStatusInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one successful HTTP status compatible with this accepted local task, method, and media types.",
		"The complete response contains the registered schema and exactly one success_status: 200, 201, 202, or 204.",
	)
}

func ApplicationServiceEndpointSuccessStatusResponseSchema(
	input ApplicationServiceEndpointSuccessStatusInput,
) (map[string]any, error) {
	candidates, err := ApplicationServiceEndpointSuccessStatusCandidates(input)
	if err != nil {
		return nil, err
	}
	return serviceEndpointLeafSchema(
		ApplicationServiceEndpointSuccessStatusSchemaV1, "success_status",
		map[string]any{"type": "integer", "enum": candidates},
	), nil
}

func DecodeApplicationServiceEndpointSuccessStatusResult(
	input ApplicationServiceEndpointSuccessStatusInput,
	raw string,
) (ApplicationServiceEndpointSuccessStatusResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointSuccessStatusResult{}, err
	}
	var result ApplicationServiceEndpointSuccessStatusResult
	return decodeApplicationServiceEndpointLeaf(raw, &result, func(value ApplicationServiceEndpointSuccessStatusResult) error {
		return value.ValidateFor(input)
	})
}
