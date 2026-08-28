package assemblyline

import (
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

func (input GroundedAnswerInput) Validate() error {
	return input.validate()
}

func (input GroundedAnswerInput) validate() error {
	if err := validateGroundedID("requirement ID", input.RequirementID, maxGroundedRequirementIDBytes); err != nil {
		return err
	}
	return validateGroundedAnswerAuthority(
		input.ExactRequirement, input.Context, input.Evidence,
	)
}

func validateGroundedAnswerAuthority(
	exactRequirement string,
	context ObjectiveContext,
	evidence []GroundedEvidenceCapsule,
) error {
	if err := validateGroundedText(
		"exact requirement", exactRequirement, maxGroundedRequirementBytes, false,
	); err != nil {
		return err
	}
	if err := context.Validate(); err != nil {
		return err
	}
	if len(evidence) < 1 || len(evidence) > maxGroundedEvidenceCapsules {
		return fmt.Errorf(
			"grounded answer requires between 1 and %d evidence capsules", maxGroundedEvidenceCapsules,
		)
	}
	seenIDs := make(map[string]struct{}, len(evidence))
	seenText := make(map[string]struct{}, len(evidence))
	total := 0
	for index, capsule := range evidence {
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
