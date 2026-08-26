package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BuildApplicationServiceEndpointRequirementPrompt(
	input ApplicationServiceEndpointRequirementInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode service endpoint requirement authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Classify whether this one accepted local service task's own behavior requires a direct HTTP request and response interaction.",
		"Choose endpoint_required when an HTTP requester directly invokes or retrieves this task's behavior. Choose support_only when this task's own behavior has no direct HTTP interaction.",
		"Return the registered schema and exactly one endpoint_requirement value.",
		"ACCEPTED_LOCAL_TASK_AUTHORITY_JSON:\n" + string(authority),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"service endpoint requirement prompt exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func ApplicationServiceEndpointRequirementResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "endpoint_requirement"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationServiceEndpointRequirementSchemaV1,
			},
			"endpoint_requirement": map[string]any{
				"type": "string", "enum": []string{
					string(ApplicationServiceEndpointRequired),
					string(ApplicationServiceSupportOnly),
				},
			},
		},
	)
}

func DecodeApplicationServiceEndpointRequirementResult(
	input ApplicationServiceEndpointRequirementInput,
	raw string,
) (ApplicationServiceEndpointRequirementResult, error) {
	var result ApplicationServiceEndpointRequirementResult
	if err := input.validate(); err != nil {
		return result, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return result, fmt.Errorf(
			"service endpoint requirement result exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("decode service endpoint requirement result: %w", err)
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointRequirementResult{}, err
	}
	return result, nil
}
