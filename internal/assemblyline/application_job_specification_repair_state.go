package assemblyline

import (
	"fmt"
	"reflect"
)

func ApplyApplicationJobSpecificationRepair(
	input ApplicationJobSpecificationRepairInput,
	retained ApplicationJobSpecification,
	patch ApplicationJobSpecificationRepairPatch,
) (ApplicationJobSpecification, error) {
	if err := input.validate(); err != nil {
		return ApplicationJobSpecification{}, err
	}
	if !reflect.DeepEqual(retained, input.retained) {
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification repair retained authority drifted",
		)
	}
	if patch.field != input.review.Field || patch.current != input.review.FindingEvidence ||
		applicationJobSpecificationRepairIsNoOp(retained, patch) {
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification repair patch retargeted immutable authority",
		)
	}
	updated := cloneApplicationJobSpecification(retained)
	switch patch.field {
	case ApplicationJobSpecificationObjectiveField:
		updated.Objective = patch.replacement
	case ApplicationJobSpecificationRequiredBehaviorsField:
		var err error
		updated.RequiredBehaviors, err = applyApplicationJobSpecificationListRepair(
			updated.RequiredBehaviors, input.review.FindingEvidence, patch,
		)
		if err != nil {
			return ApplicationJobSpecification{}, err
		}
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		var err error
		updated.AcceptanceCriteria, err = applyApplicationJobSpecificationListRepair(
			updated.AcceptanceCriteria, input.review.FindingEvidence, patch,
		)
		if err != nil {
			return ApplicationJobSpecification{}, err
		}
	default:
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification repair field %q is unsupported", patch.field,
		)
	}
	if err := ValidateApplicationJobSpecification(updated); err != nil {
		return ApplicationJobSpecification{}, err
	}
	return updated, nil
}

func applyApplicationJobSpecificationListRepair(
	values []string,
	current string,
	patch ApplicationJobSpecificationRepairPatch,
) ([]string, error) {
	index := -1
	for candidateIndex, value := range values {
		if value == current {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("application job specification repair value is no longer retained")
	}
	updated := append([]string(nil), values...)
	updated[index] = patch.replacement
	return updated, nil
}

func applicationJobSpecificationRepairIsNoOp(
	retained ApplicationJobSpecification,
	patch ApplicationJobSpecificationRepairPatch,
) bool {
	switch patch.field {
	case ApplicationJobSpecificationObjectiveField:
		return retained.Objective == patch.replacement
	case ApplicationJobSpecificationRequiredBehaviorsField,
		ApplicationJobSpecificationAcceptanceCriteriaField:
		return patch.replacement == patch.current
	default:
		return true
	}
}
