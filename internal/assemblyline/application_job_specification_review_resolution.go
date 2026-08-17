package assemblyline

import "fmt"

func applicationJobSpecificationReviewCanRemove(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) bool {
	switch field {
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return len(retained.RequiredBehaviors) > 1
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return len(retained.AcceptanceCriteria) > 1
	default:
		return false
	}
}

func validateApplicationJobSpecificationBoundReview(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
) error {
	if err := validateApplicationJobSpecificationInput(authority); err != nil {
		return err
	}
	if err := ValidateApplicationJobSpecification(retained); err != nil {
		return fmt.Errorf("application job specification review requires valid retained state: %w", err)
	}
	if review.Decision != ApplicationJobSpecificationReviewRemove &&
		review.Decision != ApplicationJobSpecificationReviewReplace {
		return fmt.Errorf("application job specification state change requires remove or replace review")
	}
	if !isApplicationJobSpecificationField(review.Field) {
		return fmt.Errorf("application job specification review field %q is unsupported", review.Field)
	}
	observed, err := applicationJobSpecificationCurrentFieldSHA256(retained, review.Field)
	if err != nil {
		return err
	}
	if review.observedValueSHA256 != observed {
		return fmt.Errorf("application job specification review is not bound to current named field")
	}
	if !applicationJobSpecificationReviewEvidenceApplies(retained, review) {
		return fmt.Errorf("application job specification review evidence does not apply to current named field")
	}
	binding, err := applicationJobSpecificationBinding(authority, retained)
	if err != nil {
		return err
	}
	if review.binding != binding {
		return fmt.Errorf("application job specification review is not bound to retained authority")
	}
	return nil
}

// ApplyApplicationJobSpecificationReviewRemoval deletes exactly the bound
// list value selected by a validated semantic review.
func ApplyApplicationJobSpecificationReviewRemoval(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
) (ApplicationJobSpecification, error) {
	if err := validateApplicationJobSpecificationBoundReview(authority, retained, review); err != nil {
		return ApplicationJobSpecification{}, err
	}
	if review.Decision != ApplicationJobSpecificationReviewRemove ||
		!applicationJobSpecificationReviewCanRemove(retained, review.Field) {
		return ApplicationJobSpecification{}, fmt.Errorf("application job specification review removal is unavailable")
	}
	updated := cloneApplicationJobSpecification(retained)
	var err error
	switch review.Field {
	case ApplicationJobSpecificationRequiredBehaviorsField:
		updated.RequiredBehaviors, err = removeApplicationJobSpecificationListValue(updated.RequiredBehaviors, review.FindingEvidence)
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		updated.AcceptanceCriteria, err = removeApplicationJobSpecificationListValue(updated.AcceptanceCriteria, review.FindingEvidence)
	default:
		err = fmt.Errorf("application job specification field %q cannot be removed", review.Field)
	}
	if err != nil {
		return ApplicationJobSpecification{}, err
	}
	if err := ValidateApplicationJobSpecification(updated); err != nil {
		return ApplicationJobSpecification{}, err
	}
	return updated, nil
}

// ApplyApplicationJobSpecificationReviewReplacement replaces exactly the
// code-bound current leaf with the reviewed candidate value.
func ApplyApplicationJobSpecificationReviewReplacement(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
) (ApplicationJobSpecification, error) {
	if err := validateApplicationJobSpecificationBoundReview(authority, retained, review); err != nil {
		return ApplicationJobSpecification{}, err
	}
	if review.Decision != ApplicationJobSpecificationReviewReplace {
		return ApplicationJobSpecification{}, fmt.Errorf("application job specification review replacement requires replace decision")
	}
	updated := cloneApplicationJobSpecification(retained)
	switch review.Field {
	case ApplicationJobSpecificationObjectiveField:
		updated.Objective = review.ReplacementValue
	case ApplicationJobSpecificationRequiredBehaviorsField:
		var err error
		updated.RequiredBehaviors, err = replaceApplicationJobSpecificationListValue(
			updated.RequiredBehaviors, review.FindingEvidence, review.ReplacementValue,
		)
		if err != nil {
			return ApplicationJobSpecification{}, err
		}
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		var err error
		updated.AcceptanceCriteria, err = replaceApplicationJobSpecificationListValue(
			updated.AcceptanceCriteria, review.FindingEvidence, review.ReplacementValue,
		)
		if err != nil {
			return ApplicationJobSpecification{}, err
		}
	default:
		return ApplicationJobSpecification{}, fmt.Errorf("application job specification field %q is unsupported", review.Field)
	}
	if err := ValidateApplicationJobSpecification(updated); err != nil {
		return ApplicationJobSpecification{}, err
	}
	return updated, nil
}

func removeApplicationJobSpecificationListValue(values []string, current string) ([]string, error) {
	for index, value := range values {
		if value == current {
			updated := append([]string(nil), values...)
			return append(updated[:index], updated[index+1:]...), nil
		}
	}
	return nil, fmt.Errorf("application job specification review value is no longer retained")
}

func replaceApplicationJobSpecificationListValue(values []string, current, replacement string) ([]string, error) {
	for index, value := range values {
		if value == current {
			updated := append([]string(nil), values...)
			updated[index] = replacement
			return updated, nil
		}
	}
	return nil, fmt.Errorf("application job specification review value is no longer retained")
}
