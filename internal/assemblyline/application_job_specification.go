package assemblyline

import "fmt"

func validateApplicationJobSpecificationInput(input ApplicationJobSpecificationInput) error {
	if err := validateApplicationAuthority(
		"application job specification", input.Surface, input.ProductQuote, input.AcceptedRequirements,
	); err != nil {
		return err
	}
	focused := -1
	for index, requirement := range input.AcceptedRequirements {
		if requirement.ID == input.FocusedRequirement.ID {
			focused = index
			break
		}
	}
	if focused < 0 {
		return fmt.Errorf("application job specification focused requirement is not accepted")
	}
	if input.FocusedRequirement != input.AcceptedRequirements[focused] {
		return fmt.Errorf("application job specification focused requirement differs from accepted authority")
	}
	return input.ValidatePathFree(ArtifactIdentityProvenance{})
}

// ValidatePathFree applies current-tree provenance at the final model-call
// boundary. Exact known bare artifact names are code-owned identities even
// though the qualified-path grammar intentionally preserves other dotted
// semantic names.
func (input ApplicationJobSpecificationInput) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	values := []string{input.ProductQuote, input.FocusedRequirement.SourceQuote}
	for _, requirement := range input.AcceptedRequirements {
		values = append(values, requirement.SourceQuote)
	}
	return ValidatePathFreeModelContextWithProvenance(
		"application job specification input", provenance, values...,
	)
}

func FirstApplicationJobSpecificationDefect(
	specification ApplicationJobSpecification,
) *ApplicationJobSpecificationDefect {
	if err := validateApplicationWorkloadLine(
		"objective", specification.Objective, maxApplicationObjectiveRunes,
	); err != nil {
		return applicationJobSpecificationDefectAt(
			ApplicationJobSpecificationObjectiveField, "objective", err,
		)
	}
	if defect := firstApplicationJobSpecificationListDefect(
		"required behavior", specification.RequiredBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
		ApplicationJobSpecificationRequiredBehaviorsField,
	); defect != nil {
		return defect
	}
	if defect := firstApplicationJobSpecificationListDefect(
		"acceptance criterion", specification.AcceptanceCriteria,
		maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
		ApplicationJobSpecificationAcceptanceCriteriaField,
	); defect != nil {
		return defect
	}
	return nil
}

func ValidateApplicationJobSpecification(specification ApplicationJobSpecification) error {
	if defect := FirstApplicationJobSpecificationDefect(specification); defect != nil {
		return defect
	}
	return nil
}

// ValidatePathFree is the provenance-aware acceptance boundary for a complete
// model-authored job specification.
func (specification ApplicationJobSpecification) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	values := []string{specification.Objective}
	values = append(values, specification.RequiredBehaviors...)
	values = append(values, specification.AcceptanceCriteria...)
	return ValidatePathFreeModelContextWithProvenance(
		"application job specification", provenance, values...,
	)
}

func firstApplicationJobSpecificationListDefect(
	label string,
	values []string,
	maximumCount int,
	maximumRunes int,
	field ApplicationJobSpecificationField,
) *ApplicationJobSpecificationDefect {
	if len(values) < 1 || len(values) > maximumCount {
		return applicationJobSpecificationDefectAt(
			field, "", fmt.Errorf("requires 1..%d %ss", maximumCount, label),
		)
	}
	seen := make(map[string]int, len(values))
	for index, value := range values {
		target := fmt.Sprintf("%s_%03d", field, index+1)
		if err := validateApplicationWorkloadLine(label, value, maximumRunes); err != nil {
			return applicationJobSpecificationDefectAt(
				field, target, fmt.Errorf("%s %d: %w", label, index, err),
			)
		}
		if earlier, duplicate := seen[value]; duplicate {
			return applicationJobSpecificationDefectAt(
				field, target,
				fmt.Errorf("%s %d duplicates earlier item %d", label, index, earlier),
			)
		}
		seen[value] = index
	}
	return nil
}

func applicationJobSpecificationDefectAt(
	field ApplicationJobSpecificationField,
	target string,
	err error,
) *ApplicationJobSpecificationDefect {
	return &ApplicationJobSpecificationDefect{
		Field: field, Detail: err.Error(), correctionTarget: target,
	}
}
