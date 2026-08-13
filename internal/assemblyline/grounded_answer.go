package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	GroundedAnswerSchemaV1        = "omnidex.grounded-answer.v1"
	maxGroundedRequirementIDBytes = 128
	maxGroundedRequirementBytes   = 4 * 1024
	maxGroundedEvidenceCapsules   = 12
	maxGroundedEvidenceIDBytes    = 128
	maxGroundedEvidenceTextBytes  = 2 * 1024
	maxGroundedEvidenceTotalBytes = 8 * 1024
	maxGroundedAnswerTextBytes    = 4 * 1024
)

type GroundedEvidenceCapsule struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type GroundedAnswerInput struct {
	RequirementID    string                    `json:"requirement_id"`
	ExactRequirement string                    `json:"exact_requirement"`
	Context          ObjectiveContext          `json:"objective_context"`
	Evidence         []GroundedEvidenceCapsule `json:"evidence"`
}

type GroundedAnswerDecision struct {
	Schema        string   `json:"schema"`
	RequirementID string   `json:"requirement_id"`
	Text          string   `json:"text"`
	EvidenceIDs   []string `json:"evidence_ids"`
}

func NewGroundedAnswerJob(input GroundedAnswerInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkGroundedAnswer, input, input.validate)
}

func (input GroundedAnswerInput) validate() error {
	if err := validateGroundedID("requirement ID", input.RequirementID, maxGroundedRequirementIDBytes); err != nil {
		return err
	}
	if err := validateGroundedText(
		"exact requirement", input.ExactRequirement, maxGroundedRequirementBytes, false,
	); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if len(input.Evidence) < 1 || len(input.Evidence) > maxGroundedEvidenceCapsules {
		return fmt.Errorf(
			"grounded answer requires between 1 and %d evidence capsules", maxGroundedEvidenceCapsules,
		)
	}
	seenIDs := make(map[string]struct{}, len(input.Evidence))
	seenText := make(map[string]struct{}, len(input.Evidence))
	total := 0
	for index, capsule := range input.Evidence {
		if err := validateGroundedID("evidence ID", capsule.ID, maxGroundedEvidenceIDBytes); err != nil {
			return fmt.Errorf("grounded evidence capsule %d: %w", index, err)
		}
		if _, duplicate := seenIDs[capsule.ID]; duplicate {
			return fmt.Errorf("grounded evidence ID %q is duplicated", capsule.ID)
		}
		seenIDs[capsule.ID] = struct{}{}
		if err := validateGroundedText(
			"evidence text", capsule.Text, maxGroundedEvidenceTextBytes, false,
		); err != nil {
			return fmt.Errorf("grounded evidence capsule %s: %w", capsule.ID, err)
		}
		if _, duplicate := seenText[capsule.Text]; duplicate {
			return fmt.Errorf("grounded evidence capsule %s duplicates evidence text", capsule.ID)
		}
		seenText[capsule.Text] = struct{}{}
		total += len(capsule.ID) + len(capsule.Text)
	}
	if total > maxGroundedEvidenceTotalBytes {
		return fmt.Errorf("grounded evidence exceeds %d total bytes", maxGroundedEvidenceTotalBytes)
	}
	return nil
}

func (decision GroundedAnswerDecision) ValidateFor(input GroundedAnswerInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != GroundedAnswerSchemaV1 {
		return fmt.Errorf("grounded answer schema must be %q", GroundedAnswerSchemaV1)
	}
	if decision.RequirementID != input.RequirementID {
		return fmt.Errorf(
			"grounded answer requirement ID %q does not match %q",
			decision.RequirementID, input.RequirementID,
		)
	}
	if err := validateGroundedText("answer text", decision.Text, maxGroundedAnswerTextBytes, true); err != nil {
		return err
	}
	if len(decision.EvidenceIDs) < 1 || len(decision.EvidenceIDs) > len(input.Evidence) {
		return fmt.Errorf("grounded answer must cite between 1 and %d evidence IDs", len(input.Evidence))
	}
	available := make(map[string]struct{}, len(input.Evidence))
	for _, capsule := range input.Evidence {
		available[capsule.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.EvidenceIDs))
	for index, id := range decision.EvidenceIDs {
		if err := validateGroundedID("cited evidence ID", id, maxGroundedEvidenceIDBytes); err != nil {
			return fmt.Errorf("grounded answer evidence ID %d: %w", index, err)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("grounded answer evidence ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
		if _, exists := available[id]; !exists {
			return fmt.Errorf("grounded answer evidence ID %q was not projected", id)
		}
	}
	return nil
}

func DecodeGroundedAnswerDecision(
	input GroundedAnswerInput,
	raw string,
) (GroundedAnswerDecision, error) {
	if len(raw) > maxPortableCandidateBytes {
		return GroundedAnswerDecision{}, fmt.Errorf(
			"grounded answer candidate exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	var decision GroundedAnswerDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return GroundedAnswerDecision{}, fmt.Errorf("decode grounded answer decision: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return GroundedAnswerDecision{}, err
	}
	return decision, nil
}

func BuildGroundedAnswerPrompt(input GroundedAnswerInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode grounded answer projection: %w", err)
	}
	return strings.Join([]string{
		"Answer exactly one requirement using only the supplied evidence capsules.",
		"Return the answer text and the opaque IDs of every capsule used. Do not invent evidence, perform work, or add another objective.",
		"GROUNDING_PROJECTION_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func GroundedAnswerResponseSchema(input GroundedAnswerInput) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	evidenceIDs := make([]string, 0, len(input.Evidence))
	for _, capsule := range input.Evidence {
		evidenceIDs = append(evidenceIDs, capsule.ID)
	}
	return objectSchema(
		[]string{"schema", "requirement_id", "text", "evidence_ids"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": GroundedAnswerSchemaV1},
			"requirement_id": map[string]any{
				"type": "string", "const": input.RequirementID,
			},
			"text": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxGroundedAnswerTextBytes,
			},
			"evidence_ids": map[string]any{
				"type": "array", "minItems": 1, "maxItems": len(evidenceIDs),
				"uniqueItems": true,
				"items":       map[string]any{"type": "string", "enum": evidenceIDs},
			},
		},
	), nil
}

func validateGroundedID(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("grounded answer %s must be one non-empty trimmed line", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("grounded answer %s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("grounded answer %s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("grounded answer %s contains NUL", label)
	}
	return nil
}

func validateGroundedText(label, value string, maximum int, trimmed bool) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("grounded answer %s is empty", label)
	}
	if trimmed && value != strings.TrimSpace(value) {
		return fmt.Errorf("grounded answer %s must be trimmed", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("grounded answer %s exceeds %d bytes", label, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("grounded answer %s is not valid UTF-8", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("grounded answer %s contains NUL", label)
	}
	return nil
}
