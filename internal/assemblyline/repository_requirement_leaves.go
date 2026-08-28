package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkRepositoryRequirementCoverage WorkKind = "repository_requirement_coverage"
	WorkRepositoryRequirement         WorkKind = "repository_requirement"

	RepositoryRequirementRemains     = "REQUIREMENT_REMAINS"
	RepositoryNoUncoveredRequirement = "NO_UNCOVERED_REQUIREMENT"
	MaxRepositoryRequirementLeaves   = maxRequirementCount
)

type RepositoryRequirementLeafInput struct {
	Authority            RepositoryRequirementInterpretationInput `json:"authority"`
	AcceptedRequirements []string                                 `json:"accepted_requirements"`
}

func NewRepositoryRequirementCoverageJob(
	input RepositoryRequirementLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkRepositoryRequirementCoverage, input, input.validate,
	)
}

func NewRepositoryRequirementJob(
	input RepositoryRequirementLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkRepositoryRequirement, input, input.validate)
}

func (input RepositoryRequirementLeafInput) validate() error {
	if err := input.Authority.validate(); err != nil {
		return err
	}
	if input.AcceptedRequirements == nil {
		return fmt.Errorf("repository requirement leaf requires a non-nil accepted set")
	}
	if len(input.AcceptedRequirements) > maxRequirementCount {
		return fmt.Errorf(
			"repository requirement leaf exceeds %d accepted requirements",
			maxRequirementCount,
		)
	}
	seen := make(map[string]struct{}, len(input.AcceptedRequirements))
	for index, requirement := range input.AcceptedRequirements {
		if err := validateRepositoryRequirementStatement(
			fmt.Sprintf("accepted repository requirement %d", index), requirement,
		); err != nil {
			return err
		}
		if _, duplicate := seen[requirement]; duplicate {
			return fmt.Errorf("accepted repository requirement %d is duplicated", index)
		}
		seen[requirement] = struct{}{}
	}
	return nil
}

func BuildRepositoryRequirementCoveragePrompt(
	input RepositoryRequirementLeafInput,
) (string, error) {
	authority, err := repositoryRequirementLeafAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: does the immutable existing-repository request establish any explicit workspace-change requirement that is not semantically covered by the accepted requirement statements?",
		"Return REQUIREMENT_REMAINS when at least one explicit change remains uncovered. Return NO_UNCOVERED_REQUIREMENT when every explicit change requirement is covered.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"REPOSITORY_REQUIREMENT_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeRepositoryRequirementCoverageLeaf(
	input RepositoryRequirementLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf("repository requirement coverage", raw, 32, false)
	if err != nil {
		return "", err
	}
	if leaf != RepositoryRequirementRemains &&
		leaf != RepositoryNoUncoveredRequirement {
		return "", fmt.Errorf("repository requirement coverage value %q is not registered", leaf)
	}
	return leaf, nil
}

func BuildRepositoryRequirementPrompt(
	input RepositoryRequirementLeafInput,
) (string, error) {
	authority, err := repositoryRequirementLeafAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one explicit workspace-change requirement from the immutable existing-repository request that is not semantically covered by the accepted requirement statements.",
		"Choose the earliest uncovered requirement in source order. Faithfully paraphrase only that one requirement without paths, implementation guesses, or added obligations.",
		"Return only the requirement as raw prose. Do not return another requirement, JSON, quotes, a label, Markdown, or commentary.",
		"REPOSITORY_REQUIREMENT_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeRepositoryRequirementLeaf(
	input RepositoryRequirementLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"repository requirement", raw, maxRequirementQuoteBytes, true,
	)
	if err != nil {
		return "", err
	}
	if err := validateRepositoryRequirementStatement("repository requirement", leaf); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedRequirements {
		if leaf == accepted {
			return "", fmt.Errorf("repository requirement duplicates an accepted statement")
		}
	}
	return leaf, nil
}

func repositoryRequirementLeafAuthority(
	input RepositoryRequirementLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode repository requirement leaf authority: %w", err)
	}
	return string(raw), nil
}
