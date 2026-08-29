package assemblyline

import (
	"fmt"
	"strings"
)

const PlainTextArtifactCreationSchemaV1 = "omnidex.plain-text-artifact-creation.v1"

type PlainTextArtifactCreationRelation string

const (
	OneNewCompletePlainTextArtifactRequired PlainTextArtifactCreationRelation = "one_new_complete_plain_text_artifact_required"
	PlainTextArtifactCreationNotExplicit    PlainTextArtifactCreationRelation = "plain_text_artifact_creation_not_explicit"
)

type PlainTextArtifactCreationInput struct {
	RequirementQuote string `json:"requirement_quote"`
}

type PlainTextArtifactCreationDecision struct {
	Schema   string                            `json:"schema"`
	Relation PlainTextArtifactCreationRelation `json:"relation"`
}

func NewPlainTextArtifactCreationJob(
	input PlainTextArtifactCreationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkPlainTextArtifactCreation, input, input.validate)
}

func (input PlainTextArtifactCreationInput) validate() error {
	if err := validateRequirementQuote(
		"plain-text artifact creation", input.RequirementQuote,
	); err != nil {
		return err
	}
	if len(input.RequirementQuote) > maxDeclarationBoundaryQuoteBytes {
		return fmt.Errorf(
			"plain-text artifact creation quote exceeds %d bytes",
			maxDeclarationBoundaryQuoteBytes,
		)
	}
	return nil
}

func (decision PlainTextArtifactCreationDecision) ValidateFor(
	input PlainTextArtifactCreationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != PlainTextArtifactCreationSchemaV1 {
		return fmt.Errorf(
			"plain-text artifact creation schema must be %q",
			PlainTextArtifactCreationSchemaV1,
		)
	}
	switch decision.Relation {
	case OneNewCompletePlainTextArtifactRequired, PlainTextArtifactCreationNotExplicit:
		return nil
	default:
		return fmt.Errorf(
			"plain-text artifact creation relation %q is unsupported",
			decision.Relation,
		)
	}
}

func BuildPlainTextArtifactCreationPrompt(
	input PlainTextArtifactCreationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: does the exact cohesive requirement explicitly call for exactly one new complete unstructured plain-text artifact and no other change?",
		"Return one_new_complete_plain_text_artifact_required only when the requirement supplies the artifact's complete literal or natural-language body and requires no source code, configuration, structured data, software behavior, or additional change. Return plain_text_artifact_creation_not_explicit otherwise.",
		"Return exactly one raw registered relation with no JSON, quotes, label, Markdown, or commentary.",
		"EXACT_REQUIREMENT:\n" + input.RequirementQuote,
	}, "\n\n"), nil
}

func DecodePlainTextArtifactCreationDecision(
	input PlainTextArtifactCreationInput,
	raw string,
) (PlainTextArtifactCreationDecision, error) {
	if err := input.validate(); err != nil {
		return PlainTextArtifactCreationDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf("plain-text artifact creation", raw, 64, false)
	if err != nil {
		return PlainTextArtifactCreationDecision{}, err
	}
	decision := PlainTextArtifactCreationDecision{
		Schema:   PlainTextArtifactCreationSchemaV1,
		Relation: PlainTextArtifactCreationRelation(leaf),
	}
	if err := decision.ValidateFor(input); err != nil {
		return PlainTextArtifactCreationDecision{}, err
	}
	return decision, nil
}
