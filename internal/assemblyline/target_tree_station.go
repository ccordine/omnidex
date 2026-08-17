package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NewTargetTreeJob(input TargetTreeInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationTargetTree, input, input.Validate)
}

func BuildTargetTreePrompt(input TargetTreeInput) (string, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}
	requirements, err := json.Marshal(input.Requirements)
	if err != nil {
		return "", fmt.Errorf("encode target tree requirements: %w", err)
	}
	current, err := json.Marshal(input.Current)
	if err != nil {
		return "", fmt.Errorf("encode target tree current inventory: %w", err)
	}
	prompt := strings.Join([]string{
		"Declare the smallest complete desired file structure for one accepted software objective.",
		"Return artifact nodes only. Each accepted requirement needs exactly one implementation artifact and exactly one verification artifact. Each node names a desired normalized relative file path, its kind, purpose, and the accepted requirement IDs it serves. Preserve an existing opaque artifact ID when that exact existing artifact belongs in the target tree; use one new key only for a genuinely new artifact.",
		"The response is a data declaration. Use only the response fields and no explanatory prose.",
		"ACCEPTED_OBJECTIVE:\n" + input.Objective,
		"ACCEPTED_REQUIREMENTS_JSON:\n" + string(requirements),
		"CURRENT_ARTIFACT_INVENTORY_JSON:\n" + string(current),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("target tree prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func TargetTreeResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "artifacts"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": TargetTreeCandidateSchemaV1},
			"artifacts": map[string]any{
				"type": "array", "minItems": 1, "maxItems": maxTargetTreeArtifacts,
				"items": objectSchema(
					[]string{"path", "kind", "purpose", "requirement_ids", "existing_artifact_id", "new_key"},
					map[string]any{
						"path":                 map[string]any{"type": "string", "minLength": 1, "maxLength": maxTargetTreePathBytes},
						"kind":                 map[string]any{"type": "string", "enum": []string{string(TargetArtifactImplementation), string(TargetArtifactVerification)}},
						"purpose":              map[string]any{"type": "string", "minLength": 1, "maxLength": maxTargetTreePurposeBytes},
						"requirement_ids":      map[string]any{"type": "array", "minItems": 1, "maxItems": maxTargetTreeRequirements, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}},
						"existing_artifact_id": map[string]any{"type": "string", "maxLength": 128},
						"new_key":              map[string]any{"type": "string", "maxLength": 128},
					},
				),
			},
		},
	)
}

func DecodeTargetTreeCandidate(input TargetTreeInput, raw string) (TargetTree, error) {
	var zero TargetTree
	if err := input.Validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("target tree candidate exceeds %d bytes", maxPortableCandidateBytes)
	}
	var candidate TargetTreeCandidate
	if err := decodePortablePayload([]byte(raw), &candidate); err != nil {
		return zero, fmt.Errorf("decode target tree candidate: %w", err)
	}
	target, err := candidate.ValidateFor(input)
	if err != nil {
		return zero, err
	}
	return target, nil
}
