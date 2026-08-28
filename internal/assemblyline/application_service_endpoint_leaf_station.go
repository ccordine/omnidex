package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildApplicationServiceEndpointLeafPrompt(
	authority any,
	question string,
	response string,
) (string, error) {
	raw, err := json.Marshal(authority)
	if err != nil {
		return "", fmt.Errorf("encode service endpoint leaf authority: %w", err)
	}
	prompt := strings.Join([]string{
		question,
		response,
		"Return only that raw semantic leaf with no JSON, quotes, label, Markdown, or commentary.",
		"LOCAL_ACCEPTED_AUTHORITY_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("service endpoint leaf prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
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
