package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BuildApplicationServiceStateLifetimePrompt(
	input ApplicationServiceStateLifetimeInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode service state lifetime authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Classify whether this one accepted local service behavior requires authoritative state produced during one request or process to remain available to a later request or process.",
		"Choose cross_request_authority_required only when that later availability is necessary. Choose request_local_only when the behavior can be satisfied within the current request or process, including stateless behavior.",
		"Return the registered schema and exactly one state_lifetime value.",
		"ACCEPTED_LOCAL_BEHAVIOR_AUTHORITY_JSON:\n" + string(authority),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"service state lifetime prompt exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func ApplicationServiceStateLifetimeResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "state_lifetime"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationServiceStateLifetimeSchemaV1,
			},
			"state_lifetime": map[string]any{
				"type": "string", "enum": []string{
					string(ApplicationServiceStateRequestLocalOnly),
					string(ApplicationServiceStateCrossRequestAuthorityRequired),
				},
			},
		},
	)
}

func DecodeApplicationServiceStateLifetimeResult(
	input ApplicationServiceStateLifetimeInput,
	raw string,
) (ApplicationServiceStateLifetimeResult, error) {
	var result ApplicationServiceStateLifetimeResult
	if err := input.validate(); err != nil {
		return result, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return result, fmt.Errorf(
			"service state lifetime result exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("decode service state lifetime result: %w", err)
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceStateLifetimeResult{}, err
	}
	return result, nil
}
