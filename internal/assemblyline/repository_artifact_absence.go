package assemblyline

import (
	"fmt"
	"strings"
)

const RepositoryArtifactAbsenceSchemaV1 = "omnidex.repository-artifact-absence.v1"

type RepositoryArtifactAbsenceRelation string

const (
	RepositoryArtifactMustBeAbsent       RepositoryArtifactAbsenceRelation = "repository_artifact_must_be_absent"
	RepositoryArtifactAbsenceNotExplicit RepositoryArtifactAbsenceRelation = "repository_artifact_absence_not_explicit"
)

type RepositoryArtifactAbsenceInput struct {
	RequirementQuote string `json:"requirement_quote"`
}

type RepositoryArtifactAbsenceDecision struct {
	Schema   string                            `json:"schema"`
	Relation RepositoryArtifactAbsenceRelation `json:"relation"`
}

func NewRepositoryArtifactAbsenceJob(
	input RepositoryArtifactAbsenceInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkRepositoryArtifactAbsence, input, input.validate)
}

func (input RepositoryArtifactAbsenceInput) validate() error {
	if err := validateRequirementQuote(
		"repository artifact absence", input.RequirementQuote,
	); err != nil {
		return err
	}
	if len(input.RequirementQuote) > maxDeclarationBoundaryQuoteBytes {
		return fmt.Errorf(
			"repository artifact absence quote exceeds %d bytes",
			maxDeclarationBoundaryQuoteBytes,
		)
	}
	return nil
}

func (decision RepositoryArtifactAbsenceDecision) ValidateFor(
	input RepositoryArtifactAbsenceInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RepositoryArtifactAbsenceSchemaV1 {
		return fmt.Errorf(
			"repository artifact absence schema must be %q",
			RepositoryArtifactAbsenceSchemaV1,
		)
	}
	switch decision.Relation {
	case RepositoryArtifactMustBeAbsent, RepositoryArtifactAbsenceNotExplicit:
		return nil
	default:
		return fmt.Errorf(
			"repository artifact absence relation %q is unsupported",
			decision.Relation,
		)
	}
}

func BuildRepositoryArtifactAbsencePrompt(
	input RepositoryArtifactAbsenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: does the exact requirement explicitly require one semantic artifact established by repository authority, including all behavior it owns, to be absent?",
		"Return repository_artifact_must_be_absent only when that complete absence is explicit. Return repository_artifact_absence_not_explicit for modification, advice, partial behavior removal, creation, or any other requirement.",
		"Return exactly one raw registered relation with no JSON, quotes, label, Markdown, or commentary.",
		"EXACT_REQUIREMENT:\n" + input.RequirementQuote,
	}, "\n\n"), nil
}

func DecodeRepositoryArtifactAbsenceDecision(
	input RepositoryArtifactAbsenceInput,
	raw string,
) (RepositoryArtifactAbsenceDecision, error) {
	if err := input.validate(); err != nil {
		return RepositoryArtifactAbsenceDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf("repository artifact absence", raw, 64, false)
	if err != nil {
		return RepositoryArtifactAbsenceDecision{}, err
	}
	decision := RepositoryArtifactAbsenceDecision{
		Schema:   RepositoryArtifactAbsenceSchemaV1,
		Relation: RepositoryArtifactAbsenceRelation(leaf),
	}
	if err := decision.ValidateFor(input); err != nil {
		return RepositoryArtifactAbsenceDecision{}, err
	}
	return decision, nil
}
