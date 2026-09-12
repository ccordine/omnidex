package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/modelcontext"
)

const (
	WorkGroundedParagraphRelevance WorkKind = "grounded_paragraph_relevance"
	WorkGroundedParagraphSupport   WorkKind = "grounded_paragraph_support"

	GroundedParagraphRelevant    GroundedParagraphRelevance = "RELEVANT"
	GroundedParagraphNotRelevant GroundedParagraphRelevance = "NOT_RELEVANT"

	GroundedParagraphFullySupported    GroundedParagraphSupport = "FULLY_SUPPORTED"
	GroundedParagraphNotFullySupported GroundedParagraphSupport = "NOT_FULLY_SUPPORTED"

	GroundedAllFactualClaims GroundedClaimDomain = "all_factual_claims"
	GroundedRealWorldClaims  GroundedClaimDomain = "real_world_factual_claims"

	maxGroundedParagraphRelationBytes = 2 * 1024
)

type GroundedParagraphRelevance string
type GroundedParagraphSupport string
type GroundedClaimDomain string

// Relevance has no evidence or character-style authority. It asks only whether
// one candidate addresses the question, using context to resolve its meaning.
type GroundedParagraphRelevanceInput struct {
	ExactQuestion      string           `json:"exact_question"`
	Context            ObjectiveContext `json:"objective_context"`
	ParagraphText      string           `json:"paragraph_text"`
	KnownArtifactPaths []string         `json:"known_artifact_paths"`
}

// Support has no question, continuity, or voice authority. The code-owned claim
// domain distinguishes factual answers from real-world claims in fictional prose.
type GroundedParagraphSupportInput struct {
	ParagraphText      string                    `json:"paragraph_text"`
	Evidence           []GroundedEvidenceCapsule `json:"evidence"`
	ClaimDomain        GroundedClaimDomain       `json:"claim_domain"`
	KnownArtifactPaths []string                  `json:"known_artifact_paths"`
}

func (input GroundedParagraphRelevanceInput) validate() error {
	if err := validateGroundedText("exact question", input.ExactQuestion, maxGroundedRequirementBytes, false); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	values := []string{input.ExactQuestion}
	for _, capsule := range input.Context.Capsules {
		values = append(values, capsule.Content)
	}
	return validateGroundedParagraphRelationInput(input.ParagraphText, input.KnownArtifactPaths, values...)
}

func (input GroundedParagraphSupportInput) validate() error {
	if _, err := input.ClaimDomain.description(); err != nil {
		return err
	}
	if err := validateGroundedEvidenceCapsules(input.Evidence); err != nil {
		return err
	}
	return validateGroundedParagraphRelationInput(
		input.ParagraphText, input.KnownArtifactPaths, groundedAnswerEvidenceText(input.Evidence)...,
	)
}

func validateGroundedParagraphRelationInput(paragraph string, paths []string, values ...string) error {
	if err := validateGroundedText("paragraph", paragraph, maxGroundedParagraphRelationBytes, true); err != nil {
		return err
	}
	if strings.ContainsAny(paragraph, "\r\n") {
		return fmt.Errorf("grounded paragraph relation requires one single-line paragraph")
	}
	if modelAuthoredCitationSyntax.MatchString(paragraph) {
		return fmt.Errorf("grounded paragraph relation contains model-authored citation syntax")
	}
	pathsView, err := modelcontext.NewArtifactIdentityProvenance(paths)
	if err != nil {
		return err
	}
	return ValidatePathFreeModelContextWithProvenance(
		"grounded paragraph relation", pathsView, append(values, paragraph)...,
	)
}

func (domain GroundedClaimDomain) description() (string, error) {
	switch domain {
	case GroundedAllFactualClaims:
		return "factual claim", nil
	case GroundedRealWorldClaims:
		return "real-world factual claim", nil
	default:
		return "", fmt.Errorf("grounded paragraph claim domain %q is not registered", domain)
	}
}
