package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	RoleplayGroundedResponseSchemaV1  = "omnidex.roleplay-grounded-response.v1"
	maxRoleplayGroundedEvidence       = 4
	maxRoleplayGroundedParagraphs     = 4
	maxRoleplayGroundedParagraphBytes = 2 * 1024
)

type RoleplayGroundedResponseInput struct {
	ExactQuestion      string                     `json:"exact_question"`
	RoleplayIdentity   RoleplayResponseIdentity   `json:"roleplay_identity"`
	RoleplayUserTurn   RoleplayUserTurnProjection `json:"roleplay_user_turn"`
	Context            ObjectiveContext           `json:"objective_context"`
	RealWorldEvidence  []GroundedEvidenceCapsule  `json:"real_world_evidence"`
	KnownArtifactPaths []string                   `json:"known_artifact_paths"`
}

type RoleplayGroundedParagraph struct {
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type RoleplayGroundedResponseDecision struct {
	Schema     string                      `json:"schema"`
	Paragraphs []RoleplayGroundedParagraph `json:"paragraphs"`
}

func AssembleRoleplayGroundedResponseDecision(
	input RoleplayGroundedResponseInput,
	paragraphs []RoleplayGroundedParagraph,
) (RoleplayGroundedResponseDecision, error) {
	cloned := make([]RoleplayGroundedParagraph, len(paragraphs))
	for index, paragraph := range paragraphs {
		cloned[index] = paragraph
		cloned[index].EvidenceIDs = append([]string(nil), paragraph.EvidenceIDs...)
	}
	decision := RoleplayGroundedResponseDecision{
		Schema: RoleplayGroundedResponseSchemaV1, Paragraphs: cloned,
	}
	if err := decision.ValidateFor(input); err != nil {
		return RoleplayGroundedResponseDecision{}, err
	}
	return decision, nil
}

func (input RoleplayGroundedResponseInput) Validate() error {
	return input.validate()
}

func (input RoleplayGroundedResponseInput) validate() error {
	if err := validateRoleplayGroundedSemanticAuthority(
		input.ExactQuestion,
		input.RoleplayIdentity,
		input.Context,
		input.RealWorldEvidence,
		input.KnownArtifactPaths,
	); err != nil {
		return err
	}
	if err := input.RoleplayUserTurn.validate(); err != nil {
		return err
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(input.KnownArtifactPaths)
	if err != nil {
		return err
	}
	values := []string{
		input.RoleplayUserTurn.PersonaName,
		input.RoleplayUserTurn.PersonaSummary,
	}
	for _, part := range input.RoleplayUserTurn.Parts {
		values = append(values, part.Text)
	}
	return ValidatePathFreeModelContextWithProvenance(
		"roleplay grounded user-turn authority", provenance, values...,
	)
}

func validateRoleplayGroundedSemanticAuthority(
	exactQuestion string,
	identity RoleplayResponseIdentity,
	context ObjectiveContext,
	evidence []GroundedEvidenceCapsule,
	knownArtifactPaths []string,
) error {
	if err := validateGroundedText(
		"roleplay grounded question", exactQuestion, roleplay.MaxResearchQuestionBytes, true,
	); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"character name":    identity.CharacterName,
		"character summary": identity.Summary,
	} {
		if err := validateContextText("roleplay "+label, value, 1024); err != nil {
			return err
		}
	}
	if err := validateOptionalContextText(
		"roleplay character voice", identity.Voice, 1024,
	); err != nil {
		return err
	}
	if len(evidence) < 1 || len(evidence) > maxRoleplayGroundedEvidence {
		return fmt.Errorf("roleplay grounded response requires 1..%d evidence capsules", maxRoleplayGroundedEvidence)
	}
	if err := (GroundedAnswerInput{
		RequirementID:      "roleplay-grounded-response",
		ExactRequirement:   exactQuestion,
		Context:            CloneObjectiveContext(context),
		Evidence:           append([]GroundedEvidenceCapsule(nil), evidence...),
		KnownArtifactPaths: append([]string{}, knownArtifactPaths...),
	}).validate(); err != nil {
		return err
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(knownArtifactPaths)
	if err != nil {
		return err
	}
	return ValidatePathFreeModelContextWithProvenance(
		"roleplay grounded identity authority", provenance,
		identity.CharacterName, identity.Summary, identity.Voice,
	)
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
		if err := validateRoleplayGroundedParagraphText(paragraph.Text); err != nil {
			return fmt.Errorf("paragraph %d: %w", index, err)
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
	provenance, err := modelcontext.NewArtifactIdentityProvenance(input.KnownArtifactPaths)
	if err != nil {
		return err
	}
	for _, paragraph := range decision.Paragraphs {
		if err := ValidatePathFreeModelContextWithProvenance(
			"roleplay grounded response paragraph", provenance, paragraph.Text,
		); err != nil {
			return err
		}
	}
	return nil
}
