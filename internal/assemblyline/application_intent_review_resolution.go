package assemblyline

import "fmt"

func applicationIntentReviewCanRemove(candidate ApplicationIntentCandidate, target string) bool {
	return target != "product_context" && len(candidate.Requirements) > 1
}

func validateBoundApplicationIntentReview(
	authority ApplicationIntentInput,
	retained ApplicationIntentCandidate,
	review ApplicationIntentReview,
) error {
	if err := authority.validate(); err != nil {
		return err
	}
	if err := retained.Validate(); err != nil {
		return err
	}
	if review.Decision != ApplicationIntentReviewRemove && review.Decision != ApplicationIntentReviewReplace {
		return fmt.Errorf("application intent state change requires remove or replace review")
	}
	current, err := applicationIntentTargetValue(retained, review.Target)
	if err != nil {
		return err
	}
	if review.requestSHA256 != authority.Context.RequestSHA256 || review.valueSHA256 != ExactObjectiveContextSHA(current) || review.CurrentValue != current {
		return fmt.Errorf("application intent review is not bound to the current named value")
	}
	return nil
}

func ApplyApplicationIntentReviewRemoval(
	authority ApplicationIntentInput,
	retained ApplicationIntentCandidate,
	review ApplicationIntentReview,
) (ApplicationIntentCandidate, error) {
	if err := validateBoundApplicationIntentReview(authority, retained, review); err != nil {
		return ApplicationIntentCandidate{}, err
	}
	if review.Decision != ApplicationIntentReviewRemove || !applicationIntentReviewCanRemove(retained, review.Target) {
		return ApplicationIntentCandidate{}, fmt.Errorf("application intent review removal is unavailable")
	}
	index, err := applicationIntentRequirementTargetIndex(review.Target)
	if err != nil || index >= len(retained.Requirements) {
		return ApplicationIntentCandidate{}, fmt.Errorf("application intent review removal target %q is unavailable", review.Target)
	}
	updated := cloneApplicationIntentCandidate(retained)
	updated.Requirements = append(updated.Requirements[:index], updated.Requirements[index+1:]...)
	if err := updated.Validate(); err != nil {
		return ApplicationIntentCandidate{}, err
	}
	return updated, nil
}

func ApplyApplicationIntentReviewReplacement(
	authority ApplicationIntentInput,
	retained ApplicationIntentCandidate,
	review ApplicationIntentReview,
) (ApplicationIntentCandidate, error) {
	if err := validateBoundApplicationIntentReview(authority, retained, review); err != nil {
		return ApplicationIntentCandidate{}, err
	}
	if review.Decision != ApplicationIntentReviewReplace {
		return ApplicationIntentCandidate{}, fmt.Errorf("application intent review replacement requires replace decision")
	}
	updated := cloneApplicationIntentCandidate(retained)
	if review.Target == "product_context" {
		updated.ProductContext = review.ReplacementValue
	} else {
		index, err := applicationIntentRequirementTargetIndex(review.Target)
		if err != nil || index >= len(updated.Requirements) {
			return ApplicationIntentCandidate{}, fmt.Errorf("application intent review replacement target %q is unavailable", review.Target)
		}
		updated.Requirements[index] = review.ReplacementValue
	}
	if err := updated.Validate(); err != nil {
		return ApplicationIntentCandidate{}, err
	}
	return updated, nil
}
