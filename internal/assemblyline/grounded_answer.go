package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/modelcontext"
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

	MaxGroundedAnswerParagraphCandidates = 4
	maxGroundedAnswerParagraphBytes      = (maxGroundedAnswerTextBytes - 2*(MaxGroundedAnswerParagraphCandidates-1)) /
		MaxGroundedAnswerParagraphCandidates
)

type GroundedEvidenceCapsule struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type GroundedAnswerInput struct {
	RequirementID      string                    `json:"requirement_id"`
	ExactRequirement   string                    `json:"exact_requirement"`
	Context            ObjectiveContext          `json:"objective_context"`
	Evidence           []GroundedEvidenceCapsule `json:"evidence"`
	KnownArtifactPaths []string                  `json:"known_artifact_paths"`
}

type GroundedAnswerDecision struct {
	Schema        string   `json:"schema"`
	RequirementID string   `json:"requirement_id"`
	Text          string   `json:"text"`
	EvidenceIDs   []string `json:"evidence_ids"`
}

type GroundedAnswerParagraph struct {
	Text        string
	EvidenceIDs []string
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
		input.KnownArtifactPaths,
	)
}

func validateGroundedAnswerAuthority(
	exactRequirement string,
	context ObjectiveContext,
	evidence []GroundedEvidenceCapsule,
	knownArtifactPaths []string,
) error {
	provenance, err := modelcontext.NewArtifactIdentityProvenance(knownArtifactPaths)
	if err != nil {
		return fmt.Errorf("grounded answer artifact provenance: %w", err)
	}
	if err := validateGroundedText(
		"exact requirement", exactRequirement, maxGroundedRequirementBytes, false,
	); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContextWithProvenance(
		"grounded answer exact requirement", provenance, exactRequirement,
	); err != nil {
		return err
	}
	if err := context.Validate(); err != nil {
		return err
	}
	for _, capsule := range context.Capsules {
		if err := ValidatePathFreeModelContextWithProvenance(
			"grounded answer objective context", provenance, capsule.Content,
		); err != nil {
			return err
		}
	}
	return validateGroundedEvidenceCapsules(evidence)
}

func validateGroundedEvidenceCapsules(evidence []GroundedEvidenceCapsule) error {
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

func AssembleGroundedAnswerDecision(
	input GroundedAnswerInput,
	paragraphs []GroundedAnswerParagraph,
) (GroundedAnswerDecision, error) {
	var zero GroundedAnswerDecision
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(paragraphs) < 1 || len(paragraphs) > MaxGroundedAnswerParagraphCandidates {
		return zero, fmt.Errorf(
			"grounded answer requires between 1 and %d accepted paragraphs",
			MaxGroundedAnswerParagraphCandidates,
		)
	}
	available := make(map[string]struct{}, len(input.Evidence))
	for _, evidence := range input.Evidence {
		available[evidence.ID] = struct{}{}
	}
	texts := make([]string, 0, len(paragraphs))
	seenTexts := make(map[string]struct{}, len(paragraphs))
	evidenceIDs := make([]string, 0, len(input.Evidence))
	seenEvidence := make(map[string]struct{}, len(input.Evidence))
	for index, paragraph := range paragraphs {
		if err := validateGroundedAnswerParagraphText(
			paragraph.Text, input.KnownArtifactPaths,
		); err != nil {
			return zero, fmt.Errorf("grounded answer paragraph %d: %w", index, err)
		}
		if _, duplicate := seenTexts[paragraph.Text]; duplicate {
			return zero, fmt.Errorf("grounded answer paragraph %d duplicates accepted text", index)
		}
		seenTexts[paragraph.Text] = struct{}{}
		if len(paragraph.EvidenceIDs) < 1 || len(paragraph.EvidenceIDs) > len(input.Evidence) {
			return zero, fmt.Errorf("grounded answer paragraph %d requires projected evidence IDs", index)
		}
		paragraphSeen := make(map[string]struct{}, len(paragraph.EvidenceIDs))
		for _, id := range paragraph.EvidenceIDs {
			if _, exists := available[id]; !exists {
				return zero, fmt.Errorf("grounded answer paragraph %d cites unavailable evidence %q", index, id)
			}
			if _, duplicate := paragraphSeen[id]; duplicate {
				return zero, fmt.Errorf("grounded answer paragraph %d duplicates evidence %q", index, id)
			}
			paragraphSeen[id] = struct{}{}
			if _, retained := seenEvidence[id]; !retained {
				evidenceIDs = append(evidenceIDs, id)
				seenEvidence[id] = struct{}{}
			}
		}
		texts = append(texts, paragraph.Text)
	}
	decision := GroundedAnswerDecision{
		Schema: GroundedAnswerSchemaV1, RequirementID: input.RequirementID,
		Text: strings.Join(texts, "\n\n"), EvidenceIDs: evidenceIDs,
	}
	if err := decision.ValidateFor(input); err != nil {
		return zero, err
	}
	return decision, nil
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
	provenance, err := modelcontext.NewArtifactIdentityProvenance(input.KnownArtifactPaths)
	if err != nil {
		return fmt.Errorf("grounded answer artifact provenance: %w", err)
	}
	if err := decision.ValidatePathFree(provenance); err != nil {
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

func (decision GroundedAnswerDecision) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	return ValidatePathFreeModelContextWithProvenance(
		"grounded answer text", provenance, decision.Text,
	)
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
