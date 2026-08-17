package assemblyline

import "fmt"

type ApplicationJobSpecificationReviewDecision string

const (
	ApplicationJobSpecificationReviewAccept  ApplicationJobSpecificationReviewDecision = "accept"
	ApplicationJobSpecificationReviewRemove  ApplicationJobSpecificationReviewDecision = "remove"
	ApplicationJobSpecificationReviewReplace ApplicationJobSpecificationReviewDecision = "replace"

	maxApplicationJobSpecificationReplacementRunes = 512
)

type ApplicationJobSpecificationReview struct {
	Decision         ApplicationJobSpecificationReviewDecision `json:"decision"`
	Field            ApplicationJobSpecificationField          `json:"field,omitempty"`
	FindingEvidence  string                                    `json:"finding_evidence,omitempty"`
	ReplacementValue string                                    `json:"replacement_value,omitempty"`

	binding             string
	observedValueSHA256 string
}

type ApplicationJobSpecificationReviewInput struct {
	authority         ApplicationJobSpecificationInput
	retained          ApplicationJobSpecification
	field             ApplicationJobSpecificationField
	evidenceID        string
	attempt           int
	validationFailure *ApplicationJobSpecificationReviewEvidenceError
}

type applicationJobSpecificationReviewWire struct {
	Decision         *ApplicationJobSpecificationReviewDecision `json:"decision"`
	EvidenceID       *string                                    `json:"evidence_id"`
	ReplacementValue *string                                    `json:"replacement_value"`
}

func NewApplicationJobSpecificationReviewInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	evidenceID string,
	attempt int,
) (ApplicationJobSpecificationReviewInput, error) {
	return newApplicationJobSpecificationReviewInput(
		authority, retained, field, evidenceID, attempt, nil,
	)
}

func NewApplicationJobSpecificationReviewRetryInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	evidenceID string,
	attempt int,
	validationFailure ApplicationJobSpecificationReviewEvidenceError,
) (ApplicationJobSpecificationReviewInput, error) {
	return newApplicationJobSpecificationReviewInput(
		authority, retained, field, evidenceID, attempt, &validationFailure,
	)
}

func newApplicationJobSpecificationReviewInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	evidenceID string,
	attempt int,
	validationFailure *ApplicationJobSpecificationReviewEvidenceError,
) (ApplicationJobSpecificationReviewInput, error) {
	input := ApplicationJobSpecificationReviewInput{
		authority: authority, retained: cloneApplicationJobSpecification(retained),
		field: field, evidenceID: evidenceID, attempt: attempt,
	}
	if validationFailure != nil {
		copy := *validationFailure
		input.validationFailure = &copy
	}
	if err := input.validate(); err != nil {
		return ApplicationJobSpecificationReviewInput{}, err
	}
	return input, nil
}

func (input ApplicationJobSpecificationReviewInput) validate() error {
	if err := validateApplicationJobSpecificationInput(input.authority); err != nil {
		return err
	}
	if err := ValidateApplicationJobSpecification(input.retained); err != nil {
		return fmt.Errorf("application job specification review requires valid retained state: %w", err)
	}
	if !isApplicationJobSpecificationField(input.field) {
		return fmt.Errorf("application job specification review field %q is unsupported", input.field)
	}
	if _, exists := applicationJobSpecificationReviewEvidenceValue(
		input.retained, input.field, input.evidenceID,
	); !exists {
		return fmt.Errorf("application job specification review evidence %q is unavailable for field %q", input.evidenceID, input.field)
	}
	if input.attempt < 1 {
		return fmt.Errorf("application job specification review attempt must be positive")
	}
	if input.validationFailure != nil {
		if input.validationFailure.Field != input.field {
			return fmt.Errorf("application job specification review failure targets a different field")
		}
		if err := input.validationFailure.validateForRetry(input.authority, input.retained); err != nil {
			return err
		}
	}
	return nil
}

func DecodeApplicationJobSpecificationReview(
	input ApplicationJobSpecificationReviewInput,
	raw string,
) (ApplicationJobSpecificationReview, error) {
	var zero ApplicationJobSpecificationReview
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application job specification review exceeds %d bytes", maxPortableCandidateBytes)
	}
	var wire applicationJobSpecificationReviewWire
	if err := decodePortablePayload([]byte(raw), &wire); err != nil {
		return zero, fmt.Errorf("decode application job specification review: %w", err)
	}
	if wire.Decision == nil || wire.EvidenceID == nil || wire.ReplacementValue == nil {
		return zero, fmt.Errorf("application job specification review requires decision, evidence_id, and replacement_value")
	}
	review := ApplicationJobSpecificationReview{Decision: *wire.Decision}
	binding, err := applicationJobSpecificationBinding(input.authority, input.retained)
	if err != nil {
		return zero, err
	}
	if review.Decision == ApplicationJobSpecificationReviewAccept {
		if *wire.EvidenceID != "" || *wire.ReplacementValue != "" {
			return zero, fmt.Errorf("accepted application job specification review must not name evidence or a replacement")
		}
		review.binding = binding
		return review, nil
	}
	if review.Decision != ApplicationJobSpecificationReviewRemove &&
		review.Decision != ApplicationJobSpecificationReviewReplace {
		return zero, fmt.Errorf("application job specification review decision %q is unsupported", review.Decision)
	}
	observed, err := applicationJobSpecificationCurrentFieldSHA256(input.retained, input.field)
	if err != nil {
		return zero, err
	}
	failure := func(kind ApplicationJobSpecificationReviewEvidenceErrorKind) *ApplicationJobSpecificationReviewEvidenceError {
		return &ApplicationJobSpecificationReviewEvidenceError{
			Kind: kind, Field: input.field, EvidenceID: *wire.EvidenceID,
			ObservedValueSHA256: observed, RetainedAuthoritySHA256: binding,
		}
	}
	if *wire.EvidenceID != input.evidenceID {
		return zero, failure(ApplicationJobSpecificationReviewEvidenceInvalid)
	}
	current, exists := applicationJobSpecificationReviewEvidenceValue(
		input.retained, input.field, *wire.EvidenceID,
	)
	if !exists {
		return zero, failure(ApplicationJobSpecificationReviewEvidenceInvalid)
	}
	review.Field = input.field
	review.FindingEvidence = current
	review.binding = binding
	review.observedValueSHA256 = observed
	if review.Decision == ApplicationJobSpecificationReviewRemove {
		if *wire.ReplacementValue != "" {
			return zero, fmt.Errorf("removed application job specification review must not provide a replacement")
		}
		if !applicationJobSpecificationReviewCanRemove(input.retained, input.field) {
			return zero, fmt.Errorf("application job specification review cannot remove the final required value")
		}
		return review, nil
	}
	if err := validateApplicationJobSpecificationReplacement(
		input.field, *wire.ReplacementValue,
	); err != nil {
		return zero, err
	}
	if *wire.ReplacementValue == current {
		return zero, failure(ApplicationJobSpecificationReviewReplacementNoOp)
	}
	review.ReplacementValue = *wire.ReplacementValue
	return review, nil
}

func validateApplicationJobSpecificationReplacement(
	field ApplicationJobSpecificationField,
	value string,
) error {
	maximum := maxApplicationObjectiveRunes
	label := "objective replacement"
	if field == ApplicationJobSpecificationRequiredBehaviorsField {
		maximum, label = maxApplicationBehaviorRunes, "required behavior replacement"
	} else if field == ApplicationJobSpecificationAcceptanceCriteriaField {
		maximum, label = maxApplicationCriterionRunes, "acceptance criterion replacement"
	}
	return validateApplicationWorkloadLine(label, value, maximum)
}

func isApplicationJobSpecificationField(field ApplicationJobSpecificationField) bool {
	switch field {
	case ApplicationJobSpecificationObjectiveField,
		ApplicationJobSpecificationRequiredBehaviorsField,
		ApplicationJobSpecificationAcceptanceCriteriaField:
		return true
	default:
		return false
	}
}

func cloneApplicationJobSpecification(value ApplicationJobSpecification) ApplicationJobSpecification {
	value.RequiredBehaviors = append([]string(nil), value.RequiredBehaviors...)
	value.AcceptanceCriteria = append([]string(nil), value.AcceptanceCriteria...)
	return value
}
