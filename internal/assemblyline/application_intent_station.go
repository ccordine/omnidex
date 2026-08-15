package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BuildApplicationIntentPrompt(input ApplicationIntentInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	contextJSON, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode application intent context: %w", err)
	}
	prompt := strings.Join([]string{
		"Derive one concise product context and the smallest complete set of explicit software requirements from the immutable user request and authoritative context facts.",
		"Faithfully paraphrase meaning. Cover every explicitly requested capability, behavior, user-visible element, and constraint. Do not invent scope, architecture, files, paths, tools, dependencies, task order, implementation, or completion state.",
		"AUTHORITATIVE_CONTEXT_JSON:\n" + string(contextJSON),
		"IMMUTABLE_USER_REQUEST:\n" + input.UserRequest,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application intent prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationIntentResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "product_context", "requirements"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationIntentCandidateSchemaV1,
			},
			"product_context": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxApplicationProductBytes,
			},
			"requirements": map[string]any{
				"type": "array", "minItems": 1, "maxItems": maxRequirementCount,
				"items": map[string]any{
					"type": "string", "minLength": 1, "maxLength": maxRequirementQuoteBytes,
				},
			},
		},
	)
}

func DecodeApplicationIntentCandidate(
	input ApplicationIntentInput,
	raw string,
) (ApplicationIntentCandidate, error) {
	var zero ApplicationIntentCandidate
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application intent candidate exceeds %d bytes", maxPortableCandidateBytes)
	}
	var candidate ApplicationIntentCandidate
	if err := decodePortablePayload([]byte(raw), &candidate); err != nil {
		return zero, fmt.Errorf("decode application intent candidate: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return zero, err
	}
	return candidate, nil
}
