package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	RoleplayGroundedResponseSchemaV1  = "omnidex.roleplay-grounded-response.v1"
	maxRoleplayGroundedEvidence       = 4
	maxRoleplayGroundedParagraphs     = 4
	maxRoleplayGroundedParagraphBytes = 2 * 1024
)

type RoleplayGroundedResponseInput struct {
	ExactQuestion     string                     `json:"exact_question"`
	RoleplayIdentity  RoleplayResponseIdentity   `json:"roleplay_identity"`
	RoleplayUserTurn  RoleplayUserTurnProjection `json:"roleplay_user_turn"`
	Context           ObjectiveContext           `json:"objective_context"`
	RealWorldEvidence []GroundedEvidenceCapsule  `json:"real_world_evidence"`
}

type RoleplayGroundedParagraph struct {
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type RoleplayGroundedResponseDecision struct {
	Schema     string                      `json:"schema"`
	Paragraphs []RoleplayGroundedParagraph `json:"paragraphs"`
}

func (input RoleplayGroundedResponseInput) Validate() error {
	return input.validate()
}

func (input RoleplayGroundedResponseInput) validate() error {
	if err := validateGroundedText(
		"roleplay grounded question", input.ExactQuestion, roleplay.MaxResearchQuestionBytes, true,
	); err != nil {
		return err
	}
	identity := input.RoleplayIdentity
	userTurn := input.RoleplayUserTurn
	if err := (ConversationResponseInput{
		Kind: ObjectiveKindStory, ExactInstruction: input.ExactQuestion,
		Context: input.Context, RoleplayIdentity: &identity, RoleplayUserTurn: &userTurn,
	}).validate(); err != nil {
		return err
	}
	if len(input.RealWorldEvidence) < 1 || len(input.RealWorldEvidence) > maxRoleplayGroundedEvidence {
		return fmt.Errorf("roleplay grounded response requires 1..%d evidence capsules", maxRoleplayGroundedEvidence)
	}
	return (GroundedAnswerInput{
		RequirementID:    "roleplay-grounded-response",
		ExactRequirement: input.ExactQuestion,
		Evidence:         append([]GroundedEvidenceCapsule(nil), input.RealWorldEvidence...),
	}).validate()
}

func (decision RoleplayGroundedResponseDecision) ValidateFor(
	input RoleplayGroundedResponseInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RoleplayGroundedResponseSchemaV1 {
		return fmt.Errorf("roleplay grounded response schema must be %q", RoleplayGroundedResponseSchemaV1)
	}
	if len(decision.Paragraphs) < 1 || len(decision.Paragraphs) > maxRoleplayGroundedParagraphs {
		return fmt.Errorf("roleplay grounded response requires 1..%d paragraphs", maxRoleplayGroundedParagraphs)
	}
	available := make(map[string]struct{}, len(input.RealWorldEvidence))
	for _, item := range input.RealWorldEvidence {
		available[item.ID] = struct{}{}
	}
	for index, paragraph := range decision.Paragraphs {
		if err := validateGroundedText(
			"roleplay grounded paragraph", paragraph.Text, maxRoleplayGroundedParagraphBytes, true,
		); err != nil {
			return fmt.Errorf("paragraph %d: %w", index, err)
		}
		if webModelCitationSyntax.MatchString(paragraph.Text) {
			return fmt.Errorf("roleplay grounded paragraph %d contains model-authored citation syntax", index)
		}
		if len(paragraph.EvidenceIDs) < 1 || len(paragraph.EvidenceIDs) > len(input.RealWorldEvidence) {
			return fmt.Errorf("roleplay grounded paragraph %d requires projected evidence IDs", index)
		}
		seen := make(map[string]struct{}, len(paragraph.EvidenceIDs))
		for _, id := range paragraph.EvidenceIDs {
			if _, exists := available[id]; !exists {
				return fmt.Errorf("roleplay grounded paragraph %d cites unavailable evidence %q", index, id)
			}
			if _, duplicate := seen[id]; duplicate {
				return fmt.Errorf("roleplay grounded paragraph %d duplicates evidence %q", index, id)
			}
			seen[id] = struct{}{}
		}
	}
	return nil
}
