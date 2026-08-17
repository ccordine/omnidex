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
	existing, err := json.Marshal(input.ExistingPaths)
	if err != nil {
		return "", fmt.Errorf("encode target tree existing paths: %w", err)
	}
	directories, err := json.Marshal(input.ExistingDirs)
	if err != nil {
		return "", fmt.Errorf("encode target tree existing directories: %w", err)
	}
	sections := []string{
		"Return the normalized relative file paths needed to solve the accepted objective.",
		"Return paths only. Omitted existing paths remain untouched.",
		"ACCEPTED_OBJECTIVE:\n" + input.Objective,
		"CODE_SELECTED_TECHNICAL_CONTEXT:\n" + input.TechnicalContext,
		"EXISTING_WORKSPACE_PATHS_JSON:\n" + string(existing),
		"EXISTING_WORKSPACE_DIRECTORIES_JSON:\n" + string(directories),
	}
	if correction := input.Correction; correction != nil {
		sections = append(sections,
			"CURRENT_TARGET_TREE_CANDIDATE_JSON:\n"+correction.CandidateJSON,
			"VALIDATION_FAILURE:\n"+correction.Failure,
			"Return one complete replacement target-tree declaration that resolves the validation failure.",
		)
	}
	prompt := strings.Join(sections, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("target tree prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func TargetTreeResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "paths"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": TargetTreeCandidateSchemaV1},
			"paths": map[string]any{
				"type": "array", "minItems": 1, "maxItems": maxTargetTreePaths,
				"items": map[string]any{"type": "string", "minLength": 1, "maxLength": maxTargetTreePathBytes},
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
