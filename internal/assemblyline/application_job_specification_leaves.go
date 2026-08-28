package assemblyline

import (
	"encoding/json"
	"fmt"
)

const (
	WorkApplicationJobObjective      WorkKind = "application_job_objective"
	WorkApplicationBehaviorCoverage  WorkKind = "application_behavior_coverage"
	WorkApplicationBehavior          WorkKind = "application_behavior"
	WorkApplicationCriterionCoverage WorkKind = "application_criterion_coverage"
	WorkApplicationCriterion         WorkKind = "application_criterion"

	MaxApplicationRequiredBehaviorLeaves    = maxApplicationRequiredBehaviors
	MaxApplicationAcceptanceCriterionLeaves = maxApplicationAcceptanceCriteria

	ApplicationBehaviorRemains      = "BEHAVIOR_REMAINS"
	ApplicationNoUncoveredBehavior  = "NO_UNCOVERED_BEHAVIOR"
	ApplicationCriterionRemains     = "CRITERION_REMAINS"
	ApplicationNoUncoveredCriterion = "NO_UNCOVERED_CRITERION"
)

type ApplicationJobBehaviorLeafInput struct {
	Authority         ApplicationJobSpecificationInput `json:"authority"`
	Objective         string                           `json:"objective"`
	AcceptedBehaviors []string                         `json:"accepted_behaviors"`
}

type ApplicationJobCriterionLeafInput struct {
	Authority         ApplicationJobSpecificationInput `json:"authority"`
	Objective         string                           `json:"objective"`
	RequiredBehaviors []string                         `json:"required_behaviors"`
	AcceptedCriteria  []string                         `json:"accepted_criteria"`
}

func NewApplicationJobObjectiveJob(
	input ApplicationJobSpecificationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationJobObjective, input,
		func() error { return validateApplicationJobSpecificationInput(input) },
	)
}

func NewApplicationBehaviorCoverageJob(
	input ApplicationJobBehaviorLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationBehaviorCoverage, input, input.validate,
	)
}

func NewApplicationBehaviorJob(
	input ApplicationJobBehaviorLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationBehavior, input, input.validate)
}

func NewApplicationCriterionCoverageJob(
	input ApplicationJobCriterionLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationCriterionCoverage, input, input.validate,
	)
}

func NewApplicationCriterionJob(
	input ApplicationJobCriterionLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationCriterion, input, input.validate)
}

func (input ApplicationJobBehaviorLeafInput) validate() error {
	if err := validateApplicationJobSpecificationInput(input.Authority); err != nil {
		return err
	}
	if err := validateApplicationWorkloadLine(
		"application job objective", input.Objective, maxApplicationObjectiveRunes,
	); err != nil {
		return err
	}
	return validateAcceptedApplicationJobLeaves(
		"application job behavior", input.AcceptedBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
	)
}

func (input ApplicationJobCriterionLeafInput) validate() error {
	if err := validateApplicationJobSpecificationInput(input.Authority); err != nil {
		return err
	}
	if err := validateApplicationWorkloadLine(
		"application job objective", input.Objective, maxApplicationObjectiveRunes,
	); err != nil {
		return err
	}
	if err := validateApplicationJobSpecificationList(
		"required behavior", input.RequiredBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
	); err != nil {
		return fmt.Errorf("application criterion authority: %w", err)
	}
	return validateAcceptedApplicationJobLeaves(
		"application acceptance criterion", input.AcceptedCriteria,
		maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
	)
}

func validateAcceptedApplicationJobLeaves(
	label string,
	values []string,
	maximumCount int,
	maximumRunes int,
) error {
	if values == nil {
		return fmt.Errorf("%s accepted set must be non-nil", label)
	}
	if len(values) > maximumCount {
		return fmt.Errorf("%s accepted set exceeds %d leaves", label, maximumCount)
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := validateApplicationWorkloadLine(label, value, maximumRunes); err != nil {
			return fmt.Errorf("accepted %s %d: %w", label, index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("accepted %s %d is duplicated", label, index)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func applicationJobSpecificationLeafAuthority(
	input ApplicationJobSpecificationInput,
	objective string,
	requiredBehaviors []string,
	acceptedCriteria []string,
) (string, error) {
	projection := struct {
		Surface            ApplicationSurface `json:"surface"`
		ProductContext     string             `json:"product_context"`
		FocusedRequirement string             `json:"focused_requirement"`
		Objective          string             `json:"objective,omitempty"`
		RequiredBehaviors  []string           `json:"required_behaviors,omitempty"`
		AcceptedCriteria   []string           `json:"accepted_criteria,omitempty"`
	}{
		Surface: input.Surface, ProductContext: input.ProductQuote,
		FocusedRequirement: input.FocusedRequirement.SourceQuote,
		Objective:          objective,
		RequiredBehaviors:  append([]string(nil), requiredBehaviors...),
		AcceptedCriteria:   append([]string(nil), acceptedCriteria...),
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job leaf authority: %w", err)
	}
	return string(raw), nil
}
