package assemblyline

import (
	"fmt"
	"strings"
)

func BuildApplicationCriterionCoveragePrompt(
	input ApplicationJobCriterionLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := applicationJobSpecificationLeafAuthority(
		input.Authority, input.Objective, input.RequiredBehaviors, input.AcceptedCriteria,
	)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: is any specific observable acceptance condition still needed to prove the required behaviors, objective, and focused requirement but not semantically covered by the accepted criteria?",
		"Return CRITERION_REMAINS when one or more acceptance conditions remain uncovered. Return NO_UNCOVERED_CRITERION when the accepted criteria collectively prove every required behavior.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_CRITERION_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationCriterionCoverageLeaf(
	input ApplicationJobCriterionLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf("application criterion coverage", raw, 32, false)
	if err != nil {
		return "", err
	}
	switch leaf {
	case ApplicationCriterionRemains, ApplicationNoUncoveredCriterion:
		return leaf, nil
	default:
		return "", fmt.Errorf("application criterion coverage value %q is not registered", leaf)
	}
}

func BuildApplicationCriterionPrompt(
	input ApplicationJobCriterionLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := applicationJobSpecificationLeafAuthority(
		input.Authority, input.Objective, input.RequiredBehaviors, input.AcceptedCriteria,
	)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one specific observable acceptance condition needed to prove the required behaviors, objective, and focused requirement that is not semantically covered by the accepted criteria.",
		"Choose the earliest logically necessary uncovered condition. It must describe observable success without inventing arbitrary numeric precision.",
		"Return only that criterion as one raw line. Do not return another criterion, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_CRITERION_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationCriterionLeaf(
	input ApplicationJobCriterionLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application acceptance criterion", raw, maxPortableCandidateBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationWorkloadLine(
		"application acceptance criterion", leaf, maxApplicationCriterionRunes,
	); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedCriteria {
		if leaf == accepted {
			return "", fmt.Errorf("application acceptance criterion duplicates an accepted criterion")
		}
	}
	return leaf, nil
}
