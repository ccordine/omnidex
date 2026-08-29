package assemblyline

import (
	"fmt"
	"strings"
)

func BuildApplicationServiceEndpointRequirementPrompt(
	input ApplicationServiceEndpointRequirementInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	prompt := strings.Join([]string{
		"Classify whether the exact endpoint requirement needs a direct HTTP request-and-response interaction.",
		"Choose endpoint_required when an HTTP requester directly invokes or retrieves the required behavior. Choose support_only when the required behavior supports another interaction and has no direct HTTP interaction.",
		"Return exactly one raw endpoint_requirement value: endpoint_required or support_only.",
		"Return only that registered value with no JSON, quotes, label, Markdown, or commentary.",
		"PRODUCT CONTEXT:\n" + input.ProductContext,
		"EXACT ENDPOINT REQUIREMENT:\n" + input.RequirementQuote,
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
