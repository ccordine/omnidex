package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BuildApplicationServiceStateInterfacePrompt(
	input ApplicationServiceStateInterfaceInput,
) (string, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode service state interface authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Define the one minimal durable JSON data interface required by these directly related local service behaviors.",
		"Return only root fields that must be shared across requests. Every field is optional in an initially empty state. Use record_list for a collection of uniform records and give it only scalar record fields.",
		"Return the registered schema and exactly one bounded fields array. Every field must include record_fields; use an empty array unless its kind is record_list.",
		"DIRECTLY_RELATED_LOCAL_BEHAVIOR_AUTHORITY_JSON:\n" + string(authority),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"service state interface prompt exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func ApplicationServiceStateInterfaceResponseSchema() map[string]any {
	scalarKinds := []string{
		string(ApplicationServiceStateString), string(ApplicationServiceStateInteger),
		string(ApplicationServiceStateNumber), string(ApplicationServiceStateBoolean),
	}
	allKinds := append(append([]string(nil), scalarKinds...),
		string(ApplicationServiceStateStringList), string(ApplicationServiceStateIntegerList),
		string(ApplicationServiceStateNumberList), string(ApplicationServiceStateBooleanList),
		string(ApplicationServiceStateRecordList),
	)
	recordField := objectSchema(
		[]string{"name", "kind"},
		map[string]any{
			"name": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,47}$"},
			"kind": map[string]any{"type": "string", "enum": scalarKinds},
		},
	)
	field := objectSchema(
		[]string{"name", "kind", "record_fields"},
		map[string]any{
			"name": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,47}$"},
			"kind": map[string]any{"type": "string", "enum": allKinds},
			"record_fields": map[string]any{
				"type": "array", "maxItems": maxApplicationServiceStateInterfaceFields,
				"items": recordField,
			},
		},
	)
	return objectSchema(
		[]string{"schema", "fields"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationServiceStateInterfaceSchemaV1,
			},
			"fields": map[string]any{
				"type": "array", "minItems": 1,
				"maxItems": maxApplicationServiceStateInterfaceFields, "items": field,
			},
		},
	)
}

func DecodeApplicationServiceStateInterfaceResult(
	input ApplicationServiceStateInterfaceInput,
	raw string,
) (ApplicationServiceStateInterfaceResult, error) {
	var result ApplicationServiceStateInterfaceResult
	if err := input.Validate(); err != nil {
		return result, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return result, fmt.Errorf(
			"service state interface result exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("decode service state interface result: %w", err)
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceStateInterfaceResult{}, err
	}
	return result, nil
}
