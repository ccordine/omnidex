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
	reusable, err := json.Marshal(input.ReusablePaths)
	if err != nil {
		return "", fmt.Errorf("encode target tree reusable paths: %w", err)
	}
	reserved, err := json.Marshal(input.ReservedPaths)
	if err != nil {
		return "", fmt.Errorf("encode target tree reserved paths: %w", err)
	}
	constraints, err := json.Marshal(input.Constraints)
	if err != nil {
		return "", fmt.Errorf("encode target tree constraints: %w", err)
	}
	directories, err := json.Marshal(input.ExistingDirs)
	if err != nil {
		return "", fmt.Errorf("encode target tree existing directories: %w", err)
	}
	sections := []string{
		"Return the normalized relative file paths needed to solve the accepted objective.",
		"Return paths only. Every returned path must be relative to the workspace root and must not start with a slash.",
		"CODE_SELECTED_PATH_CONSTRAINTS_JSON:\n" + string(constraints),
		"Every returned path and the complete path set must satisfy CODE_SELECTED_TECHNICAL_CONTEXT exactly.",
		"ACCEPTED_OBJECTIVE:\n" + input.Objective,
		"CODE_SELECTED_TECHNICAL_CONTEXT:\n" + input.TechnicalContext,
	}
	sections = append(sections,
		"Never return a path listed in FORBIDDEN_OUTPUT_PATHS_JSON, including a path also listed as existing or reusable.",
		"FORBIDDEN_OUTPUT_PATHS_JSON:\n"+string(reserved),
		"Existing workspace paths may be returned when they need changes; omitted existing paths remain untouched.",
		"Reusable accepted paths may be returned when this objective shares them.",
		"EXISTING_WORKSPACE_PATHS_JSON:\n"+string(existing),
		"REUSABLE_ACCEPTED_PATHS_JSON:\n"+string(reusable),
		"EXISTING_WORKSPACE_DIRECTORIES_JSON:\n"+string(directories),
	)
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

func TargetTreeResponseSchema(input TargetTreeInput) (map[string]any, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	items := map[string]any{
		"type": "string", "minLength": 1, "maxLength": maxTargetTreePathBytes,
	}
	if input.Constraints.RootFilesOnly {
		// Ollama 0.24.0's schema converter selects pattern instead of
		// maxLength when both are present. Embed the hard length bound in
		// the pattern so constrained decoding cannot exhaust the context.
		items["pattern"] = fmt.Sprintf(`^[^/]{1,%d}$`, maxTargetTreePathBytes)
	}
	return objectSchema(
		[]string{"schema", "paths"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": TargetTreeCandidateSchemaV1},
			"paths": map[string]any{
				"type": "array", "minItems": input.Constraints.ExactPathCount,
				"maxItems": input.Constraints.ExactPathCount, "items": items,
			},
		},
	), nil
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
