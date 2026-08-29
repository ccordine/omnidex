package assemblyline

import "fmt"

const (
	ApplicationServiceEndpointExposureSchemaV1      = "omnidex.application-service-endpoint-exposure.v1"
	ApplicationServiceEndpointMethodSchemaV1        = "omnidex.application-service-endpoint-method.v1"
	ApplicationServiceEndpointRouteTemplateSchemaV1 = "omnidex.application-service-endpoint-route-template.v1"
	ApplicationServiceEndpointRequestMediaSchemaV1  = "omnidex.application-service-endpoint-request-media.v1"
	ApplicationServiceEndpointResponseMediaSchemaV1 = "omnidex.application-service-endpoint-response-media.v1"
	ApplicationServiceEndpointSuccessStatusSchemaV1 = "omnidex.application-service-endpoint-success-status.v1"
)

type ApplicationServiceEndpointExposureResult struct {
	Schema   string                             `json:"schema"`
	Exposure ApplicationServiceEndpointExposure `json:"exposure"`
}

type ApplicationServiceEndpointMethodResult struct {
	Schema string                           `json:"schema"`
	Method ApplicationServiceEndpointMethod `json:"method"`
}

type ApplicationServiceEndpointRouteTemplateResult struct {
	Schema        string `json:"schema"`
	RouteTemplate string `json:"route_template"`
}

type ApplicationServiceEndpointRequestMediaResult struct {
	Schema       string                          `json:"schema"`
	RequestMedia ApplicationServiceEndpointMedia `json:"request_media"`
}

type ApplicationServiceEndpointResponseMediaResult struct {
	Schema        string                          `json:"schema"`
	ResponseMedia ApplicationServiceEndpointMedia `json:"response_media"`
}

type ApplicationServiceEndpointSuccessStatusResult struct {
	Schema        string `json:"schema"`
	SuccessStatus int    `json:"success_status"`
}

func ComposeApplicationServiceEndpointContract(
	authority ApplicationServiceEndpointTaskAuthority,
	exposure ApplicationServiceEndpointExposureResult,
	method ApplicationServiceEndpointMethodResult,
	route ApplicationServiceEndpointRouteTemplateResult,
	requestMedia ApplicationServiceEndpointRequestMediaResult,
	responseMedia ApplicationServiceEndpointResponseMediaResult,
	status ApplicationServiceEndpointSuccessStatusResult,
) (ApplicationServiceEndpointContract, error) {
	exposureInput := ApplicationServiceEndpointExposureInput{Authority: authority}
	methodInput := ApplicationServiceEndpointMethodInput{Authority: authority}
	routeInput := ApplicationServiceEndpointRouteTemplateInput{Authority: authority}
	requestInput := ApplicationServiceEndpointRequestMediaInput{Authority: authority, Method: method.Method}
	responseInput := ApplicationServiceEndpointResponseMediaInput{Authority: authority, Method: method.Method}
	statusInput := ApplicationServiceEndpointSuccessStatusInput{
		Authority: authority, Method: method.Method,
		RequestMedia: requestMedia.RequestMedia, ResponseMedia: responseMedia.ResponseMedia,
	}
	validations := []struct {
		name     string
		validate func() error
	}{
		{name: "exposure", validate: func() error { return exposure.ValidateFor(exposureInput) }},
		{name: "method", validate: func() error { return method.ValidateFor(methodInput) }},
		{name: "route template", validate: func() error { return route.ValidateFor(routeInput) }},
		{name: "request media", validate: func() error { return requestMedia.ValidateFor(requestInput) }},
		{name: "response media", validate: func() error { return responseMedia.ValidateFor(responseInput) }},
		{name: "success status", validate: func() error { return status.ValidateFor(statusInput) }},
	}
	for _, validation := range validations {
		if err := validation.validate(); err != nil {
			return ApplicationServiceEndpointContract{}, fmt.Errorf(
				"compose service endpoint %s: %w", validation.name, err,
			)
		}
	}
	contract := ApplicationServiceEndpointContract{
		Schema:   ApplicationServiceEndpointContractSchemaV1,
		Exposure: exposure.Exposure, Method: method.Method, RouteTemplate: route.RouteTemplate,
		RequestMedia: requestMedia.RequestMedia, ResponseMedia: responseMedia.ResponseMedia,
		SuccessStatus: status.SuccessStatus,
	}
	if err := contract.ValidateFor(authority); err != nil {
		return ApplicationServiceEndpointContract{}, err
	}
	return contract, nil
}
