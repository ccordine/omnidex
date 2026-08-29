package assemblyline

import (
	"fmt"
	"strings"
)

func BuildApplicationServiceStateLifetimePrompt(
	input ApplicationServiceStateLifetimeInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	prompt := strings.Join([]string{
		"Classify whether the exact service requirement needs state produced during one request or process to remain available to a later request or process.",
		"Choose cross_request_authority_required only when that later availability is necessary. Choose request_local_only when the requirement can be satisfied within the current request or process, including stateless behavior.",
		"Return exactly the raw state_lifetime value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"PRODUCT CONTEXT:\n" + input.ProductContext,
		"EXACT SERVICE REQUIREMENT:\n" + input.RequirementQuote,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"service state lifetime prompt exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func DecodeApplicationServiceStateLifetimeResult(
	input ApplicationServiceStateLifetimeInput,
	raw string,
) (ApplicationServiceStateLifetimeResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceStateLifetimeResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("service state lifetime", raw, 64, false)
	if err != nil {
		return ApplicationServiceStateLifetimeResult{}, err
	}
	result := ApplicationServiceStateLifetimeResult{
		Schema:        ApplicationServiceStateLifetimeSchemaV1,
		StateLifetime: ApplicationServiceStateLifetime(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceStateLifetimeResult{}, err
	}
	return result, nil
}
