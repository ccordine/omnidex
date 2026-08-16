package assemblyline

// ApplicationJobSpecificationRepairNoOpError identifies a valid field-scoped
// repair response that did not change the authoritative field. This is a
// semantic convergence event, not a malformed-response failure.
type ApplicationJobSpecificationRepairNoOpError struct {
	Field ApplicationJobSpecificationField

	finding                 string
	findingEvidence         string
	observedValueSHA256     string
	retainedAuthoritySHA256 string
}

func (err *ApplicationJobSpecificationRepairNoOpError) Error() string {
	return "application job specification repair is a no-op"
}

func newApplicationJobSpecificationRepairNoOpError(
	review ApplicationJobSpecificationReview,
) *ApplicationJobSpecificationRepairNoOpError {
	return &ApplicationJobSpecificationRepairNoOpError{
		Field:                   review.Field,
		finding:                 review.Finding,
		findingEvidence:         review.FindingEvidence,
		observedValueSHA256:     review.observedValueSHA256,
		retainedAuthoritySHA256: review.binding,
	}
}

// ReviewFailure returns the exact rejected reviewer verdict so code can
// retain the candidate and ask the reviewer to re-evaluate that same state.
func (err *ApplicationJobSpecificationRepairNoOpError) ReviewFailure() ApplicationJobSpecificationReviewEvidenceError {
	if err == nil {
		return ApplicationJobSpecificationReviewEvidenceError{}
	}
	return ApplicationJobSpecificationReviewEvidenceError{
		Kind:                    ApplicationJobSpecificationReviewRepairNoOp,
		Field:                   err.Field,
		Finding:                 err.finding,
		FindingEvidence:         err.findingEvidence,
		ObservedValueSHA256:     err.observedValueSHA256,
		RetainedAuthoritySHA256: err.retainedAuthoritySHA256,
	}
}
