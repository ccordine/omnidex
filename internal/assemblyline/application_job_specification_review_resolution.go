package assemblyline

import "fmt"

func applicationJobSpecificationReviewResolutions(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) []ApplicationJobSpecificationReviewResolution {
	resolutions := []ApplicationJobSpecificationReviewResolution{
		ApplicationJobSpecificationReviewReplace,
	}
	if applicationJobSpecificationReviewCanRemove(retained, field) {
		resolutions = append(resolutions, ApplicationJobSpecificationReviewRemove)
	}
	return resolutions
}

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
		return fmt.Errorf(
			"application job specification review requires valid retained state: %w",
			err,
		)
	}
	if err := validateApplicationJobSpecificationReview(review); err != nil {
		return err
	}
	if review.Decision != ApplicationJobSpecificationReviewRepair {
		return fmt.Errorf("application job specification state change requires a repair review")
	}
	observedValueSHA256, err := applicationJobSpecificationCurrentFieldSHA256(
		retained, review.Field,
	)
	if err != nil {
		return err
	}
	if review.observedValueSHA256 != observedValueSHA256 {
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

// ApplyApplicationJobSpecificationReviewRemoval lets code perform an exact
// reviewer-selected list-leaf removal without asking another model to infer
// deletion from prose.
func ApplyApplicationJobSpecificationReviewRemoval(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
) (ApplicationJobSpecification, error) {
	if err := validateApplicationJobSpecificationBoundReview(authority, retained, review); err != nil {
		return ApplicationJobSpecification{}, err
	}
	if review.Resolution != ApplicationJobSpecificationReviewRemove {
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification review removal requires remove resolution",
		)
	}
	if !applicationJobSpecificationReviewCanRemove(retained, review.Field) {
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification review cannot remove the final required value",
		)
	}
	updated := cloneApplicationJobSpecification(retained)
	var err error
	switch review.Field {
	case ApplicationJobSpecificationRequiredBehaviorsField:
		updated.RequiredBehaviors, err = removeApplicationJobSpecificationListValue(
			updated.RequiredBehaviors, review.FindingEvidence,
		)
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		updated.AcceptanceCriteria, err = removeApplicationJobSpecificationListValue(
			updated.AcceptanceCriteria, review.FindingEvidence,
		)
	default:
		err = fmt.Errorf(
			"application job specification field %q cannot be removed", review.Field,
		)
	}
	if err != nil {
		return ApplicationJobSpecification{}, err
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
