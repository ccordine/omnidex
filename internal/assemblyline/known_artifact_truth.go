package assemblyline

import (
	"fmt"
	"strings"
)

const KnownArtifactTruthSchemaV1 = "omnidex.known-artifact-truth.v1"

type KnownArtifactTruth string

const (
	KnownArtifactMustBeAbsent       KnownArtifactTruth = "known_artifact_must_be_absent"
	KnownArtifactTruthNotApplicable KnownArtifactTruth = "not_applicable"
)

type KnownArtifactTruthInput struct {
	RequirementQuote string `json:"requirement_quote"`
}

type KnownArtifactTruthDecision struct {
	Schema string             `json:"schema"`
	Truth  KnownArtifactTruth `json:"truth"`
}

func NewKnownArtifactTruthJob(input KnownArtifactTruthInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkKnownArtifactTruth, input, input.validate)
}

func (input KnownArtifactTruthInput) validate() error {
	if err := validateRequirementQuote("known artifact truth", input.RequirementQuote); err != nil {
		return err
	}
	if len(input.RequirementQuote) > maxDeclarationBoundaryQuoteBytes {
		return fmt.Errorf("known artifact truth quote exceeds %d bytes", maxDeclarationBoundaryQuoteBytes)
	}
	return nil
}

func (decision KnownArtifactTruthDecision) ValidateFor(input KnownArtifactTruthInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != KnownArtifactTruthSchemaV1 {
		return fmt.Errorf("known artifact truth schema must be %q", KnownArtifactTruthSchemaV1)
	}
	switch decision.Truth {
	case KnownArtifactMustBeAbsent, KnownArtifactTruthNotApplicable:
		return nil
	default:
		return fmt.Errorf("known artifact truth %q is unsupported", decision.Truth)
	}
}

func BuildKnownArtifactTruthPrompt(input KnownArtifactTruthInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the explicit desired truth of the exact requirement.",
		"Choose known_artifact_must_be_absent only when the quote explicitly requires one semantic artifact already established by repository authority, including all behavior it owns, to be absent. Choose not_applicable for addition, modification, advice, partial behavior removal, or when that complete absence is not explicit.",
		"EXACT_REQUIREMENT:\n" + input.RequirementQuote,
	}, "\n\n"), nil
}

func KnownArtifactTruthResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "truth"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": KnownArtifactTruthSchemaV1},
			"truth":  enumSchema(KnownArtifactMustBeAbsent, KnownArtifactTruthNotApplicable),
		},
	)
}
