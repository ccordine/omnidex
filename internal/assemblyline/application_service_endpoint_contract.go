package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ApplicationServiceEndpointContractSchemaV1 = "omnidex.application-service-endpoint-contract.v1"
	maxApplicationServiceRouteBytes            = 256
)

type ApplicationServiceEndpointExposure string

const (
	ApplicationServiceEndpointPublic        ApplicationServiceEndpointExposure = "public"
	ApplicationServiceEndpointAuthenticated ApplicationServiceEndpointExposure = "authenticated"
	ApplicationServiceEndpointInternal      ApplicationServiceEndpointExposure = "internal"
)

type ApplicationServiceEndpointMethod string

const (
	ApplicationServiceEndpointGET    ApplicationServiceEndpointMethod = "GET"
	ApplicationServiceEndpointPOST   ApplicationServiceEndpointMethod = "POST"
	ApplicationServiceEndpointPUT    ApplicationServiceEndpointMethod = "PUT"
	ApplicationServiceEndpointPATCH  ApplicationServiceEndpointMethod = "PATCH"
	ApplicationServiceEndpointDELETE ApplicationServiceEndpointMethod = "DELETE"
)

type ApplicationServiceEndpointMedia string

const (
	ApplicationServiceEndpointMediaNone ApplicationServiceEndpointMedia = "none"
	ApplicationServiceEndpointJSON      ApplicationServiceEndpointMedia = "application/json"
	ApplicationServiceEndpointXML       ApplicationServiceEndpointMedia = "application/xml"
	ApplicationServiceEndpointForm      ApplicationServiceEndpointMedia = "application/x-www-form-urlencoded"
	ApplicationServiceEndpointMultipart ApplicationServiceEndpointMedia = "multipart/form-data"
	ApplicationServiceEndpointText      ApplicationServiceEndpointMedia = "text/plain"
	ApplicationServiceEndpointHTML      ApplicationServiceEndpointMedia = "text/html"
	ApplicationServiceEndpointBinary    ApplicationServiceEndpointMedia = "application/octet-stream"
)

type ApplicationServiceEndpointContract struct {
	Schema        string                             `json:"schema"`
	Exposure      ApplicationServiceEndpointExposure `json:"exposure"`
	Method        ApplicationServiceEndpointMethod   `json:"method"`
	RouteTemplate string                             `json:"route_template"`
	RequestMedia  ApplicationServiceEndpointMedia    `json:"request_media"`
	ResponseMedia ApplicationServiceEndpointMedia    `json:"response_media"`
	SuccessStatus int                                `json:"success_status"`
}

var applicationServiceRouteSegmentPattern = regexp.MustCompile(
	`^(?:[a-z0-9]+(?:-[a-z0-9]+)*|\{[a-z][a-z0-9]*(?:_[a-z0-9]+)*\})$`,
)

func (contract ApplicationServiceEndpointContract) ValidateFor(
	authority ApplicationServiceEndpointTaskAuthority,
) error {
	if err := authority.validate(); err != nil {
		return err
	}
	if contract.Schema != ApplicationServiceEndpointContractSchemaV1 {
		return fmt.Errorf(
			"service endpoint contract schema must be %q",
			ApplicationServiceEndpointContractSchemaV1,
		)
	}
	if !validApplicationServiceEndpointExposure(contract.Exposure) {
		return fmt.Errorf("service endpoint exposure %q is unsupported", contract.Exposure)
	}
	if !validApplicationServiceEndpointMethod(contract.Method) {
		return fmt.Errorf("service endpoint method %q is unsupported", contract.Method)
	}
	if err := ValidateApplicationServiceRouteTemplate(contract.RouteTemplate); err != nil {
		return err
	}
	if !validApplicationServiceRequestMedia(contract.RequestMedia) {
		return fmt.Errorf("service endpoint request media %q is unsupported", contract.RequestMedia)
	}
	if !validApplicationServiceResponseMedia(contract.ResponseMedia) {
		return fmt.Errorf("service endpoint response media %q is unsupported", contract.ResponseMedia)
	}
	if err := validateApplicationServiceSuccess(contract); err != nil {
		return err
	}
	return nil
}

func validApplicationServiceEndpointExposure(value ApplicationServiceEndpointExposure) bool {
	switch value {
	case ApplicationServiceEndpointPublic,
		ApplicationServiceEndpointAuthenticated,
		ApplicationServiceEndpointInternal:
		return true
	default:
		return false
	}
}

func validApplicationServiceEndpointMethod(value ApplicationServiceEndpointMethod) bool {
	switch value {
	case ApplicationServiceEndpointGET, ApplicationServiceEndpointPOST,
		ApplicationServiceEndpointPUT, ApplicationServiceEndpointPATCH,
		ApplicationServiceEndpointDELETE:
		return true
	default:
		return false
	}
}

func validApplicationServiceRequestMedia(value ApplicationServiceEndpointMedia) bool {
	switch value {
	case ApplicationServiceEndpointMediaNone, ApplicationServiceEndpointJSON,
		ApplicationServiceEndpointXML, ApplicationServiceEndpointForm,
		ApplicationServiceEndpointMultipart, ApplicationServiceEndpointText:
		return true
	default:
		return false
	}
}

func validApplicationServiceResponseMedia(value ApplicationServiceEndpointMedia) bool {
	switch value {
	case ApplicationServiceEndpointMediaNone, ApplicationServiceEndpointJSON,
		ApplicationServiceEndpointXML, ApplicationServiceEndpointText,
		ApplicationServiceEndpointHTML, ApplicationServiceEndpointBinary:
		return true
	default:
		return false
	}
}

// ValidateApplicationServiceRouteTemplate is the owning typed exemption for an
// intentional HTTP route. Untyped prose containing the same bytes remains
// subject to the general path-free model-context boundary.
func ValidateApplicationServiceRouteTemplate(value string) error {
	if value == "" || value != strings.TrimSpace(value) ||
		len(value) > maxApplicationServiceRouteBytes || !strings.HasPrefix(value, "/") ||
		(value != "/" && strings.HasSuffix(value, "/")) {
		return fmt.Errorf("service endpoint route template must be one normalized absolute HTTP route")
	}
	if value == "/" {
		return nil
	}
	parameters := map[string]struct{}{}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if !applicationServiceRouteSegmentPattern.MatchString(segment) {
			return fmt.Errorf("service endpoint route template segment %q is invalid", segment)
		}
		if strings.HasPrefix(segment, "{") {
			if _, duplicate := parameters[segment]; duplicate {
				return fmt.Errorf("service endpoint route template repeats parameter %q", segment)
			}
			parameters[segment] = struct{}{}
		}
	}
	return nil
}

func validateApplicationServiceSuccess(contract ApplicationServiceEndpointContract) error {
	allowed := false
	switch contract.Method {
	case ApplicationServiceEndpointGET:
		allowed = contract.SuccessStatus == 200 && contract.RequestMedia == ApplicationServiceEndpointMediaNone
	case ApplicationServiceEndpointPOST, ApplicationServiceEndpointPUT:
		allowed = contract.SuccessStatus == 200 || contract.SuccessStatus == 201 ||
			contract.SuccessStatus == 202 || contract.SuccessStatus == 204
	case ApplicationServiceEndpointPATCH, ApplicationServiceEndpointDELETE:
		allowed = contract.SuccessStatus == 200 || contract.SuccessStatus == 202 ||
			contract.SuccessStatus == 204
	}
	if !allowed {
		return fmt.Errorf(
			"service endpoint success status %d or request media is incompatible with method %q",
			contract.SuccessStatus, contract.Method,
		)
	}
	if contract.SuccessStatus == 204 && contract.ResponseMedia != ApplicationServiceEndpointMediaNone {
		return fmt.Errorf("service endpoint status 204 requires response media %q", ApplicationServiceEndpointMediaNone)
	}
	if contract.SuccessStatus != 204 && contract.ResponseMedia == ApplicationServiceEndpointMediaNone {
		return fmt.Errorf("service endpoint status %d requires response media", contract.SuccessStatus)
	}
	return nil
}
