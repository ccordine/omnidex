package assemblyline

import "fmt"

func (result ApplicationServiceEndpointExposureResult) ValidateFor(
	input ApplicationServiceEndpointExposureInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointExposureSchemaV1 {
		return fmt.Errorf("service endpoint exposure schema must be %q", ApplicationServiceEndpointExposureSchemaV1)
	}
	if !validApplicationServiceEndpointExposure(result.Exposure) {
		return fmt.Errorf("service endpoint exposure %q is unsupported", result.Exposure)
	}
	return nil
}

func (result ApplicationServiceEndpointMethodResult) ValidateFor(
	input ApplicationServiceEndpointMethodInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointMethodSchemaV1 {
		return fmt.Errorf("service endpoint method schema must be %q", ApplicationServiceEndpointMethodSchemaV1)
	}
	if !validApplicationServiceEndpointMethod(result.Method) {
		return fmt.Errorf("service endpoint method %q is unsupported", result.Method)
	}
	return nil
}

func (result ApplicationServiceEndpointRouteTemplateResult) ValidateFor(
	input ApplicationServiceEndpointRouteTemplateInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointRouteTemplateSchemaV1 {
		return fmt.Errorf("service endpoint route-template schema must be %q", ApplicationServiceEndpointRouteTemplateSchemaV1)
	}
	return ValidateApplicationServiceRouteTemplate(result.RouteTemplate)
}

func (result ApplicationServiceEndpointRequestMediaResult) ValidateFor(
	input ApplicationServiceEndpointRequestMediaInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointRequestMediaSchemaV1 {
		return fmt.Errorf("service endpoint request-media schema must be %q", ApplicationServiceEndpointRequestMediaSchemaV1)
	}
	if !validApplicationServiceRequestMedia(result.RequestMedia) {
		return fmt.Errorf("service endpoint request media %q is unsupported", result.RequestMedia)
	}
	if input.Method == ApplicationServiceEndpointGET && result.RequestMedia != ApplicationServiceEndpointMediaNone {
		return fmt.Errorf("service endpoint GET requires request media %q", ApplicationServiceEndpointMediaNone)
	}
	return nil
}

func (result ApplicationServiceEndpointResponseMediaResult) ValidateFor(
	input ApplicationServiceEndpointResponseMediaInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointResponseMediaSchemaV1 {
		return fmt.Errorf("service endpoint response-media schema must be %q", ApplicationServiceEndpointResponseMediaSchemaV1)
	}
	if !validApplicationServiceResponseMedia(result.ResponseMedia) {
		return fmt.Errorf("service endpoint response media %q is unsupported", result.ResponseMedia)
	}
	return nil
}

func (result ApplicationServiceEndpointSuccessStatusResult) ValidateFor(
	input ApplicationServiceEndpointSuccessStatusInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceEndpointSuccessStatusSchemaV1 {
		return fmt.Errorf("service endpoint success-status schema must be %q", ApplicationServiceEndpointSuccessStatusSchemaV1)
	}
	return validateApplicationServiceSuccess(ApplicationServiceEndpointContract{
		Method: input.Method, RequestMedia: input.RequestMedia,
		ResponseMedia: input.ResponseMedia, SuccessStatus: result.SuccessStatus,
	})
}

func ApplicationServiceEndpointRequestMediaCandidates(
	input ApplicationServiceEndpointRequestMediaInput,
) ([]ApplicationServiceEndpointMedia, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	candidates := make([]ApplicationServiceEndpointMedia, 0)
	for _, value := range applicationServiceRequestMediaValues() {
		candidate := ApplicationServiceEndpointMedia(value)
		result := ApplicationServiceEndpointRequestMediaResult{
			Schema: ApplicationServiceEndpointRequestMediaSchemaV1, RequestMedia: candidate,
		}
		if result.ValidateFor(input) == nil {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("service endpoint method %q has no compatible request media", input.Method)
	}
	return candidates, nil
}

func ApplicationServiceEndpointSuccessStatusCandidates(
	input ApplicationServiceEndpointSuccessStatusInput,
) ([]int, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	candidates := make([]int, 0)
	for _, candidate := range []int{200, 201, 202, 204} {
		result := ApplicationServiceEndpointSuccessStatusResult{
			Schema: ApplicationServiceEndpointSuccessStatusSchemaV1, SuccessStatus: candidate,
		}
		if result.ValidateFor(input) == nil {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf(
			"service endpoint method %q and response media %q have no compatible success status",
			input.Method, input.ResponseMedia,
		)
	}
	return candidates, nil
}
