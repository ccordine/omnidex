package assemblyline

// ApplicationIntentRepairNoOpError identifies a valid semantic replacement
// that left the exact reviewed leaf unchanged.
type ApplicationIntentRepairNoOpError struct {
	target  string
	finding string
}

func NewApplicationIntentRepairNoOpError(
	finding ApplicationIntentReviewDecision,
) *ApplicationIntentRepairNoOpError {
	return &ApplicationIntentRepairNoOpError{
		target: finding.Target, finding: finding.Finding,
	}
}

func (err *ApplicationIntentRepairNoOpError) Error() string {
	return "application intent repair is a no-op"
}

func (err *ApplicationIntentRepairNoOpError) ReviewRejection() ApplicationIntentReviewRejection {
	if err == nil {
		return ApplicationIntentReviewRejection{}
	}
	return ApplicationIntentReviewRejection{
		Target: err.target, Finding: err.finding, Reason: "repair_noop",
	}
}
