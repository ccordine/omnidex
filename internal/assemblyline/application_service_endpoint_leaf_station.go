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
		"LOCAL_ACCEPTED_AUTHORITY_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("service endpoint leaf prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func decodeApplicationServiceEndpointLeaf[T any](
	raw string,
	target *T,
	validate func(T) error,
) (T, error) {
	var zero T
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("service endpoint leaf exceeds %d bytes", maxPortableCandidateBytes)
	}
	if err := decodePortablePayload([]byte(raw), target); err != nil {
		return zero, fmt.Errorf("decode service endpoint leaf: %w", err)
	}
	if err := validate(*target); err != nil {
		return zero, err
	}
	return *target, nil
}

func serviceEndpointLeafSchema(
	schema string,
	leafName string,
	leafSchema map[string]any,
) map[string]any {
	return objectSchema(
		[]string{"schema", leafName},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": schema},
			leafName: leafSchema,
		},
	)
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
