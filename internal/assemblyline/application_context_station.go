package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const ApplicationContextNeedSchemaV1 = "omnidex.application-context-needs.v1"

func BuildApplicationContextNeedPrompt(input ApplicationContextNeedInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	contextJSON, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode application context authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Identify only evidence questions whose answers are necessary to interpret this software request faithfully and are not already answered by the authoritative context facts.",
		"Return zero questions when the supplied facts are sufficient. A question names one missing fact; it must not propose an operation, command, file, path, implementation, architecture, plan, or completion claim.",
		"AUTHORITATIVE_CONTEXT_JSON:\n" + string(contextJSON),
		"IMMUTABLE_USER_REQUEST:\n" + input.UserRequest,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application context need prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationContextNeedResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "questions"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": ApplicationContextNeedSchemaV1},
			"questions": map[string]any{
				"type": "array", "minItems": 0, "maxItems": MaxApplicationEvidenceNeeds,
				"items": map[string]any{
					"type": "string", "minLength": 1,
					"maxLength": maxApplicationEvidenceQuestionBytes,
				},
			},
		},
	)
}

func DecodeApplicationContextNeedDecision(
	input ApplicationContextNeedInput,
	raw string,
) (ApplicationContextNeedDecision, error) {
	var zero ApplicationContextNeedDecision
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application context need decision exceeds %d bytes", maxPortableCandidateBytes)
	}
	var decision ApplicationContextNeedDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return zero, fmt.Errorf("decode application context need decision: %w", err)
	}
	if err := decision.Validate(); err != nil {
		return zero, err
	}
	return decision, nil
}
