package assemblyline

import "fmt"

type applicationJobSpecificationReviewEvidence struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type ApplicationJobSpecificationReviewEvidenceErrorKind string

const (
	ApplicationJobSpecificationReviewEvidenceInvalid ApplicationJobSpecificationReviewEvidenceErrorKind = "invalid_evidence_id"
	ApplicationJobSpecificationReviewReplacementNoOp ApplicationJobSpecificationReviewEvidenceErrorKind = "replacement_noop"
)

type ApplicationJobSpecificationReviewEvidenceError struct {
	Kind                    ApplicationJobSpecificationReviewEvidenceErrorKind `json:"kind"`
	Field                   ApplicationJobSpecificationField                   `json:"field"`
	EvidenceID              string                                             `json:"evidence_id,omitempty"`
	ObservedValueSHA256     string                                             `json:"observed_value_sha256"`
	RetainedAuthoritySHA256 string                                             `json:"retained_authority_sha256"`
}

type applicationJobSpecificationReviewEvidenceFailureProjection struct {
	EvidenceID      string `json:"evidence_id,omitempty"`
	RejectedFinding string `json:"rejected_finding,omitempty"`
	Reason          string `json:"reason"`
}

func (failure *ApplicationJobSpecificationReviewEvidenceError) Error() string {
	if failure == nil {
		return "application job specification review evidence failure is unavailable"
	}
	return fmt.Sprintf(
		"application job specification review response %s for field %q: %s (observed_value_sha256=%s retained_authority_sha256=%s)",
		failure.Kind, failure.Field, failure.reason(), failure.ObservedValueSHA256,
		failure.RetainedAuthoritySHA256,
	)
}

func (failure *ApplicationJobSpecificationReviewEvidenceError) Identity() string {
	if failure == nil {
		return ""
	}
	return string(failure.Kind) + "\x00" + string(failure.Field) + "\x00" +
		failure.ObservedValueSHA256 + "\x00" +
		failure.RetainedAuthoritySHA256 + "\x00" + failure.EvidenceID
}

func (failure *ApplicationJobSpecificationReviewEvidenceError) reason() string {
	if failure == nil {
		return "evidence failure is unavailable"
	}
	switch failure.Kind {
	case ApplicationJobSpecificationReviewEvidenceInvalid:
		return "evidence_id is not listed in legal_current_evidence"
	case ApplicationJobSpecificationReviewReplacementNoOp:
		return "replacement_value was byte-identical to current_value"
	default:
		return "evidence failure kind is unsupported"
	}
}

func applicationJobSpecificationReviewEvidenceApplies(
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
) bool {
	values, err := applicationJobSpecificationReviewEvidenceValues(retained, review.Field)
	if err != nil {
		return false
	}
	for _, value := range values {
		if value == review.FindingEvidence {
			return true
		}
	}
	return false
}

func applicationJobSpecificationReviewEvidenceValues(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) ([]string, error) {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return []string{retained.Objective}, nil
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return append([]string(nil), retained.RequiredBehaviors...), nil
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return append([]string(nil), retained.AcceptanceCriteria...), nil
	default:
		return nil, fmt.Errorf("application job specification field %q is unsupported", field)
	}
}

func applicationJobSpecificationReviewEvidenceOptions(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) ([]applicationJobSpecificationReviewEvidence, error) {
	values, err := applicationJobSpecificationReviewEvidenceValues(retained, field)
	if err != nil {
		return nil, err
	}
	evidence := make([]applicationJobSpecificationReviewEvidence, 0, len(values))
	for index, value := range values {
		evidence = append(evidence, applicationJobSpecificationReviewEvidence{
			ID: fmt.Sprintf("E%d", index+1), Value: value,
		})
	}
	return evidence, nil
}

func applicationJobSpecificationReviewEvidenceValue(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	evidenceID string,
) (string, bool) {
	evidence, err := applicationJobSpecificationReviewEvidenceOptions(retained, field)
	if err != nil {
		return "", false
	}
	for _, item := range evidence {
		if item.ID == evidenceID {
			return item.Value, true
		}
	}
	return "", false
}

func applicationJobSpecificationReviewEvidenceID(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	value string,
) (string, bool) {
	evidence, err := applicationJobSpecificationReviewEvidenceOptions(retained, field)
	if err != nil {
		return "", false
	}
	for _, item := range evidence {
		if item.Value == value {
			return item.ID, true
		}
	}
	return "", false
}

// ApplicationJobSpecificationReviewEvidenceID resolves the current immutable
// evidence identity for one retained semantic leaf.
func ApplicationJobSpecificationReviewEvidenceID(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	value string,
) (string, bool) {
	return applicationJobSpecificationReviewEvidenceID(retained, field, value)
}

func (failure *ApplicationJobSpecificationReviewEvidenceError) validateForRetry(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
) error {
	if failure == nil {
		return fmt.Errorf("application job specification review retry requires evidence failure")
	}
	if !isApplicationJobSpecificationField(failure.Field) {
		return fmt.Errorf("application job specification review evidence failure field %q is unsupported", failure.Field)
	}
	want, err := applicationJobSpecificationCurrentFieldSHA256(retained, failure.Field)
	if err != nil {
		return err
	}
	if failure.ObservedValueSHA256 != want {
		return fmt.Errorf("application job specification review evidence failure is not bound to current named field")
	}
	wantAuthority, err := applicationJobSpecificationBinding(authority, retained)
	if err != nil {
		return err
	}
	if failure.RetainedAuthoritySHA256 != wantAuthority {
		return fmt.Errorf("application job specification review evidence failure is not bound to current retained authority")
	}
	switch failure.Kind {
	case ApplicationJobSpecificationReviewEvidenceInvalid:
		if failure.EvidenceID == "" {
			return fmt.Errorf("application job specification review invalid evidence failure omitted evidence identity")
		}
		if _, exists := applicationJobSpecificationReviewEvidenceValue(
			retained, failure.Field, failure.EvidenceID,
		); exists {
			return fmt.Errorf("application job specification review invalid evidence failure retained a legal evidence identity")
		}
	case ApplicationJobSpecificationReviewReplacementNoOp:
		if _, exists := applicationJobSpecificationReviewEvidenceValue(
			retained, failure.Field, failure.EvidenceID,
		); !exists {
			return fmt.Errorf("application job specification review no-op evidence is no longer current")
		}
	default:
		return fmt.Errorf("application job specification review evidence failure kind %q is unsupported", failure.Kind)
	}
	return nil
}
