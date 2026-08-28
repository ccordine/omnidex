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
		"Return exactly one raw endpoint_requirement value: endpoint_required or support_only.",
		"Return only that registered value with no JSON, quotes, label, Markdown, or commentary.",
		"ACCEPTED_LOCAL_TASK_AUTHORITY_JSON:\n" + string(authority),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"service endpoint requirement prompt exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func DecodeApplicationServiceEndpointRequirementResult(
	input ApplicationServiceEndpointRequirementInput,
	raw string,
) (ApplicationServiceEndpointRequirementResult, error) {
	var result ApplicationServiceEndpointRequirementResult
	if err := input.validate(); err != nil {
		return result, err
	}
	leaf, err := decodeRawSemanticLeaf("service endpoint requirement", raw, 64, false)
	if err != nil {
		return result, err
	}
	result = ApplicationServiceEndpointRequirementResult{
		Schema:              ApplicationServiceEndpointRequirementSchemaV1,
		EndpointRequirement: ApplicationServiceEndpointRequirement(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointRequirementResult{}, err
	}
	return result, nil
}
