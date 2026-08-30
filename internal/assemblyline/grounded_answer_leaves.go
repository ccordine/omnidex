package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
)

const (
	WorkGroundedAnswerParagraphInventory        WorkKind = "grounded_answer_paragraph_inventory"
	WorkGroundedAnswerParagraphEvidenceRelation WorkKind = "grounded_answer_paragraph_evidence_relation"
	WorkGroundedAnswerParagraphAuthorization    WorkKind = "grounded_answer_paragraph_authorization"

	GroundedEvidenceSupportsParagraph GroundedAnswerParagraphEvidenceRelation = "SUPPORTS_PARAGRAPH"
	GroundedEvidenceDoesNotSupport    GroundedAnswerParagraphEvidenceRelation = "DOES_NOT_SUPPORT_PARAGRAPH"

	GroundedParagraphResponsiveAndFullySupported GroundedAnswerParagraphAuthorization = "RESPONSIVE_AND_FULLY_SUPPORTED"
	GroundedParagraphNotResponsiveOrUnsupported  GroundedAnswerParagraphAuthorization = "NOT_RESPONSIVE_OR_NOT_FULLY_SUPPORTED"
)

type GroundedAnswerParagraphEvidenceRelation string

type GroundedAnswerParagraphAuthorization string

type GroundedAnswerParagraphInventoryInput struct {
	ExactRequirement   string                    `json:"exact_requirement"`
	Context            ObjectiveContext          `json:"objective_context"`
	Evidence           []GroundedEvidenceCapsule `json:"evidence"`
	KnownArtifactPaths []string                  `json:"known_artifact_paths"`
}

type GroundedAnswerParagraphEvidenceRelationInput struct {
	ParagraphText      string                  `json:"paragraph_text"`
	Evidence           GroundedEvidenceCapsule `json:"evidence"`
	KnownArtifactPaths []string                `json:"known_artifact_paths"`
}

type GroundedAnswerParagraphEvidenceRelationDecision struct {
	Relation GroundedAnswerParagraphEvidenceRelation `json:"relation"`
}

type GroundedAnswerParagraphAuthorizationInput struct {
	ExactRequirement   string                    `json:"exact_requirement"`
	Context            ObjectiveContext          `json:"objective_context"`
	ParagraphText      string                    `json:"paragraph_text"`
	Evidence           []GroundedEvidenceCapsule `json:"evidence"`
	KnownArtifactPaths []string                  `json:"known_artifact_paths"`
}

type GroundedAnswerParagraphAuthorizationDecision struct {
	Relation GroundedAnswerParagraphAuthorization `json:"relation"`
}

type groundedAnswerParagraphInventoryProjection struct {
	ExactRequirement  string           `json:"exact_requirement"`
	Context           ObjectiveContext `json:"objective_context"`
	Evidence          []string         `json:"evidence"`
	MaxParagraphs     int              `json:"max_paragraphs"`
	MaxParagraphBytes int              `json:"max_paragraph_bytes"`
}

type groundedAnswerParagraphEvidenceRelationProjection struct {
	ParagraphText string `json:"paragraph_text"`
	EvidenceText  string `json:"evidence_text"`
}

type groundedAnswerParagraphAuthorizationProjection struct {
	ExactRequirement string           `json:"exact_requirement"`
	Context          ObjectiveContext `json:"objective_context"`
	ParagraphText    string           `json:"paragraph_text"`
	Evidence         []string         `json:"evidence"`
}

func (input GroundedAnswerParagraphInventoryInput) validate() error {
	return validateGroundedAnswerAuthority(
		input.ExactRequirement, input.Context, input.Evidence,
		input.KnownArtifactPaths,
	)
}

func (input GroundedAnswerParagraphEvidenceRelationInput) validate() error {
	if err := validateGroundedEvidenceCapsules([]GroundedEvidenceCapsule{input.Evidence}); err != nil {
		return err
	}
	return validateGroundedAnswerParagraphText(input.ParagraphText, input.KnownArtifactPaths)
}

func (input GroundedAnswerParagraphAuthorizationInput) validate() error {
	if err := validateGroundedAnswerAuthority(
		input.ExactRequirement, input.Context, input.Evidence,
		input.KnownArtifactPaths,
	); err != nil {
		return err
	}
	return validateGroundedAnswerParagraphText(input.ParagraphText, input.KnownArtifactPaths)
}

func validateGroundedAnswerParagraphText(text string, knownArtifactPaths []string) error {
	if err := validateGroundedText(
		"paragraph text", text, maxGroundedAnswerParagraphBytes, true,
	); err != nil {
		return err
	}
	if strings.ContainsAny(text, "\r\n") {
		return fmt.Errorf("grounded answer paragraph must be one line")
	}
	if webModelCitationSyntax.MatchString(text) {
		return fmt.Errorf("grounded answer paragraph contains model-authored citation syntax")
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(knownArtifactPaths)
	if err != nil {
		return fmt.Errorf("grounded answer artifact provenance: %w", err)
	}
	return ValidatePathFreeModelContextWithProvenance(
		"grounded answer paragraph", provenance, text,
	)
}

func groundedAnswerEvidenceText(evidence []GroundedEvidenceCapsule) []string {
	result := make([]string, len(evidence))
	for index, capsule := range evidence {
		result[index] = capsule.Text
	}
	return result
}
