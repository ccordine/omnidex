package assemblyline

import (
	"fmt"
	"strings"
)

func BuildApplicationJobObjectivePrompt(
	input ApplicationJobSpecificationInput,
) (string, error) {
	if err := validateApplicationJobSpecificationInput(input); err != nil {
		return "", err
	}
	authority, err := applicationJobSpecificationLeafAuthority(input, "", nil, nil)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic question: what one independently executable local implementation objective is minimally sufficient for the focused accepted requirement?",
		"The objective must state specifically what to implement in the named product. Remain faithful to the focused requirement and do not add unrelated scope.",
		"Return only that objective as one raw line. Do not return behaviors, criteria, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_JOB_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationJobObjectiveLeaf(
	input ApplicationJobSpecificationInput,
	raw string,
) (string, error) {
	if err := validateApplicationJobSpecificationInput(input); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application job objective", raw, maxPortableCandidateBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationWorkloadLine(
		"application job objective", leaf, maxApplicationObjectiveRunes,
	); err != nil {
		return "", err
	}
	return leaf, nil
}

func BuildApplicationBehaviorCoveragePrompt(
	input ApplicationJobBehaviorLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := applicationJobSpecificationLeafAuthority(
		input.Authority, input.Objective, input.AcceptedBehaviors, nil,
	)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: is any concrete action-and-result behavior still required for the objective and focused requirement but not semantically covered by the accepted behaviors?",
		"Return BEHAVIOR_REMAINS when one or more required behaviors remain uncovered. Return NO_UNCOVERED_BEHAVIOR when the accepted behaviors minimally and sufficiently cover the objective.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_BEHAVIOR_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationBehaviorCoverageLeaf(
	input ApplicationJobBehaviorLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf("application behavior coverage", raw, 32, false)
	if err != nil {
		return "", err
	}
	switch leaf {
	case ApplicationBehaviorRemains, ApplicationNoUncoveredBehavior:
		return leaf, nil
	default:
		return "", fmt.Errorf("application behavior coverage value %q is not registered", leaf)
	}
}

func BuildApplicationBehaviorPrompt(
	input ApplicationJobBehaviorLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := applicationJobSpecificationLeafAuthority(
		input.Authority, input.Objective, input.AcceptedBehaviors, nil,
	)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one concrete action-and-result behavior required for the objective and focused requirement that is not semantically covered by the accepted behaviors.",
		"Choose the earliest logically necessary uncovered behavior. Return only that behavior as one raw line without implementation trivia or unrelated scope.",
		"Do not return another behavior, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_BEHAVIOR_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationBehaviorLeaf(
	input ApplicationJobBehaviorLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application behavior", raw, maxPortableCandidateBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationWorkloadLine(
		"application behavior", leaf, maxApplicationBehaviorRunes,
	); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedBehaviors {
		if leaf == accepted {
			return "", fmt.Errorf("application behavior duplicates an accepted behavior")
		}
	}
	return leaf, nil
}
