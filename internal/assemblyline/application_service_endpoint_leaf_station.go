package assemblyline

import (
	"fmt"
	"strings"
)

type applicationServiceEndpointPrerequisite struct {
	label string
	value string
}

func buildApplicationServiceEndpointLeafPrompt(
	authority ApplicationServiceEndpointTaskAuthority,
	prerequisites []applicationServiceEndpointPrerequisite,
	question string,
	response string,
) (string, error) {
	if err := authority.validate(); err != nil {
		return "", err
	}
	sections := []string{
		question,
		response,
		"Return only that raw semantic leaf with no JSON, quotes, label, Markdown, or commentary.",
		"PRODUCT CONTEXT:\n" + authority.ProductContext,
		"EXACT ENDPOINT REQUIREMENT:\n" + authority.RequirementQuote,
		"ACCEPTED APPLICATION SURFACE:\n" + string(authority.Surface),
	}
	for _, prerequisite := range prerequisites {
		if prerequisite.label == "" || prerequisite.value == "" {
			return "", fmt.Errorf("service endpoint leaf prerequisite is incomplete")
		}
		sections = append(sections, prerequisite.label+":\n"+prerequisite.value)
	}
	prompt := strings.Join(sections, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("service endpoint leaf prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func applicationServiceEndpointExposureValues() []string {
	return []string{
		string(ApplicationServiceEndpointPublic),
		string(ApplicationServiceEndpointAuthenticated),
		string(ApplicationServiceEndpointInternal),
	}
}

func applicationServiceEndpointMethodValues() []string {
	return []string{
		string(ApplicationServiceEndpointGET),
		string(ApplicationServiceEndpointPOST),
		string(ApplicationServiceEndpointPUT),
		string(ApplicationServiceEndpointPATCH),
		string(ApplicationServiceEndpointDELETE),
	}
}

func applicationServiceRequestMediaValues() []string {
	return []string{
		string(ApplicationServiceEndpointMediaNone), string(ApplicationServiceEndpointJSON),
		string(ApplicationServiceEndpointXML), string(ApplicationServiceEndpointForm),
		string(ApplicationServiceEndpointMultipart), string(ApplicationServiceEndpointText),
	}
}

func applicationServiceResponseMediaValues() []string {
	return []string{
		string(ApplicationServiceEndpointMediaNone), string(ApplicationServiceEndpointJSON),
		string(ApplicationServiceEndpointXML), string(ApplicationServiceEndpointText),
		string(ApplicationServiceEndpointHTML), string(ApplicationServiceEndpointBinary),
	}
}
