package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const FileContentCandidateSchemaV1 = "omnidex.file-content.v1"

// FileContentInput is one leaf after code has derived the file workload. The
// model receives no tree, queue, tools, or authority beyond this one file.
type FileContentInput struct {
	Objective      string                   `json:"objective"`
	Path           string                   `json:"path"`
	Kind           TargetArtifactKind       `json:"kind"`
	Requirements   []FileContentRequirement `json:"requirements"`
	ExistingSource string                   `json:"existing_source,omitempty"`
	Correction     *FileContentCorrection   `json:"correction,omitempty"`
}

type FileContentCorrection struct {
	CandidateJSON string `json:"candidate_json"`
	Failure       string `json:"failure"`
}

type FileContentRequirement struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

type FileContentCandidate struct {
	Schema             string `json:"schema"`
	RequirementIndexes []int  `json:"requirement_indexes"`
}

func (input FileContentInput) Validate() error {
	if err := validateTargetTreeText("file-content objective", input.Objective, maxTargetTreePathBytes); err != nil {
		return err
	}
	if err := validateTargetTreePath(input.Path); err != nil {
		return fmt.Errorf("file-content path: %w", err)
	}
	if input.Kind != TargetArtifactImplementation && input.Kind != TargetArtifactVerification {
		return fmt.Errorf("file-content kind %q is unsupported", input.Kind)
	}
	if len(input.Requirements) == 0 || len(input.Requirements) > maxTargetTreePaths {
		return fmt.Errorf("file-content requires between 1 and %d requirement statements", maxTargetTreePaths)
	}
	for index, requirement := range input.Requirements {
		if err := validateTargetTreeText("file-content requirement ID", requirement.ID, 128); err != nil {
			return fmt.Errorf("file-content requirement %d: %w", index, err)
		}
		if err := validateTargetTreeText("file-content requirement statement", requirement.Statement, maxTargetTreePathBytes); err != nil {
			return fmt.Errorf("file-content requirement %d: %w", index, err)
		}
	}
	if len(input.ExistingSource) > maxPortableCandidateBytes || strings.ContainsRune(input.ExistingSource, '\x00') {
		return fmt.Errorf("file-content existing source is invalid")
	}
	if correction := input.Correction; correction != nil {
		if err := validateTargetTreeText("file-content correction candidate", correction.CandidateJSON, maxPortableCandidateBytes); err != nil {
			return err
		}
		if err := validateTargetTreeText("file-content correction failure", correction.Failure, 1200); err != nil {
			return err
		}
	}
	return nil
}

func NewFileContentJob(input FileContentInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationFileContent, input, input.Validate)
}

func BuildFileContentPrompt(input FileContentInput) (string, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}
	requirements, err := json.Marshal(input.Requirements)
	if err != nil {
		return "", fmt.Errorf("encode file-content requirements: %w", err)
	}
	sections := []string{
		"Determine which accepted requirements exactly one file must cover.",
		"Return only the zero-based indexes of accepted requirements this file must cover.",
		"ACCEPTED_OBJECTIVE:\n" + input.Objective,
		"ACCEPTED_REQUIREMENTS_JSON:\n" + string(requirements),
	}
	if input.ExistingSource != "" {
		sections = append(sections, "EXISTING_FILE_SOURCE:\n"+input.ExistingSource)
	}
	if correction := input.Correction; correction != nil {
		sections = append(sections,
			"CURRENT_FILE_CONTENT_CANDIDATE_JSON:\n"+correction.CandidateJSON,
			"VALIDATION_FAILURE:\n"+correction.Failure,
			"Return one complete replacement file-content declaration that resolves the validation failure.",
		)
	}
	prompt := strings.Join(sections, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("file-content prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func FileContentResponseSchema() map[string]any {
	return objectSchema([]string{"schema", "requirement_indexes"}, map[string]any{
		"schema":              map[string]any{"type": "string", "const": FileContentCandidateSchemaV1},
		"requirement_indexes": map[string]any{"type": "array", "minItems": 1, "maxItems": maxTargetTreePaths, "items": map[string]any{"type": "integer", "minimum": 0}},
	})
}

func DecodeFileContentCandidate(input FileContentInput, raw string) (TargetTreeFileContent, error) {
	var zero TargetTreeFileContent
	if err := input.Validate(); err != nil {
		return zero, err
	}
	var candidate FileContentCandidate
	if err := decodePortablePayload([]byte(raw), &candidate); err != nil {
		return zero, fmt.Errorf("decode file-content candidate: %w", err)
	}
	if candidate.Schema != FileContentCandidateSchemaV1 {
		return zero, fmt.Errorf("file-content schema must be %q", FileContentCandidateSchemaV1)
	}
	if len(candidate.RequirementIndexes) == 0 || len(candidate.RequirementIndexes) > len(input.Requirements) {
		return zero, fmt.Errorf("file-content requires between 1 and %d requirement indexes", len(input.Requirements))
	}
	seen := make(map[int]struct{}, len(candidate.RequirementIndexes))
	ids := make([]string, 0, len(candidate.RequirementIndexes))
	for _, index := range candidate.RequirementIndexes {
		if index < 0 || index >= len(input.Requirements) {
			return zero, fmt.Errorf("file-content requirement index %d is outside the supplied requirement list", index)
		}
		if _, duplicate := seen[index]; duplicate {
			return zero, fmt.Errorf("file-content duplicates requirement index %d", index)
		}
		seen[index] = struct{}{}
		ids = append(ids, input.Requirements[index].ID)
	}
	return TargetTreeFileContent{Path: input.Path, Kind: input.Kind, RequirementIDs: ids}, nil
}
