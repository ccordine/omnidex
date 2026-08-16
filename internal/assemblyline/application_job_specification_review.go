package assemblyline

import (
	"fmt"
)

type ApplicationJobSpecificationReviewDecision string

const (
	ApplicationJobSpecificationReviewAccept ApplicationJobSpecificationReviewDecision = "accept"
	ApplicationJobSpecificationReviewRepair ApplicationJobSpecificationReviewDecision = "repair"

	maxApplicationJobSpecificationReviewFindingRunes         = 512
	maxApplicationJobSpecificationReviewFindingEvidenceRunes = 512
)

type ApplicationJobSpecificationReview struct {
	Decision        ApplicationJobSpecificationReviewDecision `json:"decision"`
	Field           ApplicationJobSpecificationField          `json:"field,omitempty"`
	Finding         string                                    `json:"finding,omitempty"`
	FindingEvidence string                                    `json:"finding_evidence,omitempty"`

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
	Decision   *ApplicationJobSpecificationReviewDecision `json:"decision"`
	EvidenceID *string                                    `json:"evidence_id"`
	Finding    *string                                    `json:"finding"`
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
		return fmt.Errorf(
			"application job specification review evidence %q is unavailable for field %q",
			input.evidenceID, input.field,
		)
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
	if wire.Decision == nil {
		return zero, fmt.Errorf("application job specification review requires decision")
	}
	review := ApplicationJobSpecificationReview{Decision: *wire.Decision}
	if review.Decision == ApplicationJobSpecificationReviewRepair && wire.Finding != nil {
		review.Finding = *wire.Finding
	}
	binding, err := applicationJobSpecificationBinding(input.authority, input.retained)
	if err != nil {
		return zero, err
	}
	if review.Decision == ApplicationJobSpecificationReviewRepair {
		review.Field = input.field
		if err := validateApplicationJobSpecificationReviewRepairAuthority(review); err != nil {
			return zero, err
		}
		observedValueSHA256, err := applicationJobSpecificationCurrentFieldSHA256(
			input.retained, review.Field,
		)
		if err != nil {
			return zero, err
		}
		review.observedValueSHA256 = observedValueSHA256
		failure := func(
			kind ApplicationJobSpecificationReviewEvidenceErrorKind,
			evidenceID string,
		) *ApplicationJobSpecificationReviewEvidenceError {
			return &ApplicationJobSpecificationReviewEvidenceError{
				Kind: kind, Field: review.Field, Finding: review.Finding,
				EvidenceID:          evidenceID,
				ObservedValueSHA256: observedValueSHA256, RetainedAuthoritySHA256: binding,
			}
		}
		if wire.EvidenceID == nil {
			return zero, failure(ApplicationJobSpecificationReviewEvidenceMissing, "")
		}
		if *wire.EvidenceID != input.evidenceID {
			return zero, failure(ApplicationJobSpecificationReviewEvidenceInvalid, *wire.EvidenceID)
		}
		evidenceValue, exists := applicationJobSpecificationReviewEvidenceValue(
			input.retained, input.field, *wire.EvidenceID,
		)
		if !exists {
			return zero, failure(ApplicationJobSpecificationReviewEvidenceInvalid, *wire.EvidenceID)
		}
		review.FindingEvidence = evidenceValue
	} else {
		if wire.EvidenceID == nil || wire.Finding == nil {
			return zero, fmt.Errorf(
				"accepted application job specification review requires evidence_id and finding fields",
			)
		}
		if err := validateApplicationJobSpecificationReview(review); err != nil {
			return zero, err
		}
	}
	review.binding = binding
	return review, nil
}

func validateApplicationJobSpecificationReview(review ApplicationJobSpecificationReview) error {
	switch review.Decision {
	case ApplicationJobSpecificationReviewAccept:
		if review.Field != "" || review.Finding != "" || review.FindingEvidence != "" {
			return fmt.Errorf("accepted application job specification review must not name a field, finding, or finding evidence")
		}
		return nil
	case ApplicationJobSpecificationReviewRepair:
		if err := validateApplicationJobSpecificationReviewRepairAuthority(review); err != nil {
			return err
		}
		return validateApplicationWorkloadLine(
			"application job specification review finding evidence",
			review.FindingEvidence,
			maxApplicationJobSpecificationReviewFindingEvidenceRunes,
		)
	default:
		return fmt.Errorf("application job specification review decision %q is unsupported", review.Decision)
	}
}

func validateApplicationJobSpecificationReviewRepairAuthority(
	review ApplicationJobSpecificationReview,
) error {
	if !isApplicationJobSpecificationField(review.Field) {
		return fmt.Errorf("application job specification review field %q is unsupported", review.Field)
	}
	return validateApplicationWorkloadLine(
		"application job specification review finding",
		review.Finding,
		maxApplicationJobSpecificationReviewFindingRunes,
	)
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
