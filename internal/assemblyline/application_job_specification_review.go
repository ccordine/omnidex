package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
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
	attempt           int
	validationFailure *ApplicationJobSpecificationReviewEvidenceError
}

type applicationJobSpecificationReviewWire struct {
	Decision        *ApplicationJobSpecificationReviewDecision `json:"decision"`
	Field           *ApplicationJobSpecificationField          `json:"field"`
	Finding         *string                                    `json:"finding"`
	FindingEvidence *string                                    `json:"finding_evidence"`
}

type ApplicationJobSpecificationReviewEvidenceError struct {
	Field                   ApplicationJobSpecificationField `json:"field"`
	FindingEvidence         string                           `json:"finding_evidence"`
	ObservedValueSHA256     string                           `json:"observed_value_sha256"`
	RetainedAuthoritySHA256 string                           `json:"retained_authority_sha256"`
}

type applicationJobSpecificationReviewEvidenceFailureProjection struct {
	Field                   ApplicationJobSpecificationField `json:"field"`
	FindingEvidence         string                           `json:"finding_evidence"`
	ObservedValueSHA256     string                           `json:"observed_value_sha256"`
	RetainedAuthoritySHA256 string                           `json:"retained_authority_sha256"`
	Reason                  string                           `json:"reason"`
}

func (failure *ApplicationJobSpecificationReviewEvidenceError) Error() string {
	if failure == nil {
		return "application job specification review evidence failure is unavailable"
	}
	return fmt.Sprintf(
		"application job specification review finding_evidence %q does not occur in the exact current named field %q (observed_value_sha256=%s retained_authority_sha256=%s)",
		failure.FindingEvidence, failure.Field, failure.ObservedValueSHA256,
		failure.RetainedAuthoritySHA256,
	)
}

func (failure *ApplicationJobSpecificationReviewEvidenceError) Identity() string {
	if failure == nil {
		return ""
	}
	return string(failure.Field) + "\x00" + failure.ObservedValueSHA256 + "\x00" +
		failure.RetainedAuthoritySHA256 + "\x00" + failure.FindingEvidence
}

func NewApplicationJobSpecificationReviewInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	attempt int,
) (ApplicationJobSpecificationReviewInput, error) {
	return newApplicationJobSpecificationReviewInput(authority, retained, attempt, nil)
}

func NewApplicationJobSpecificationReviewRetryInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	attempt int,
	validationFailure ApplicationJobSpecificationReviewEvidenceError,
) (ApplicationJobSpecificationReviewInput, error) {
	return newApplicationJobSpecificationReviewInput(
		authority, retained, attempt, &validationFailure,
	)
}

func newApplicationJobSpecificationReviewInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	attempt int,
	validationFailure *ApplicationJobSpecificationReviewEvidenceError,
) (ApplicationJobSpecificationReviewInput, error) {
	input := ApplicationJobSpecificationReviewInput{
		authority: authority, retained: cloneApplicationJobSpecification(retained), attempt: attempt,
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
	if input.attempt < 1 {
		return fmt.Errorf("application job specification review attempt must be positive")
	}
	if input.validationFailure != nil {
		if err := input.validationFailure.validateForRetry(input.authority, input.retained); err != nil {
			return err
		}
	}
	return nil
}

func BuildApplicationJobSpecificationReviewPrompt(
	input ApplicationJobSpecificationReviewInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var priorValidationFailure *applicationJobSpecificationReviewEvidenceFailureProjection
	if input.validationFailure != nil {
		priorValidationFailure = &applicationJobSpecificationReviewEvidenceFailureProjection{
			Field:                   input.validationFailure.Field,
			FindingEvidence:         input.validationFailure.FindingEvidence,
			ObservedValueSHA256:     input.validationFailure.ObservedValueSHA256,
			RetainedAuthoritySHA256: input.validationFailure.RetainedAuthoritySHA256,
			Reason:                  "finding_evidence does not occur in the exact current named field",
		}
	}
	projection := struct {
		UserAuthority          applicationJobSpecificationAuthority                        `json:"user_authority"`
		DerivedCandidate       ApplicationJobSpecification                                 `json:"derived_candidate"`
		PriorValidationFailure *applicationJobSpecificationReviewEvidenceFailureProjection `json:"prior_validation_failure,omitempty"`
	}{
		UserAuthority:          projectApplicationJobSpecificationAuthority(input.authority),
		DerivedCandidate:       input.retained,
		PriorValidationFailure: priorValidationFailure,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification review authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Review only whether derived_candidate is faithful to the focused user_authority and specific enough for a competent developer to implement and verify from this packet.",
		"Accept when the objective names concrete work, every required behavior names a concrete action and result, and observable acceptance criteria collectively cover those behaviors. Qualitative observable results are valid; do not demand invented numeric precision. Reject unrelated scope or capabilities, quantities, counts, ranges, timing, defaults, compatibility promises, or constraints absent from user_authority. A repair decision must identify exactly one derived field, one concise diagnostic finding that states the exact mismatch and required semantic outcome, and finding_evidence copied verbatim as one non-empty contiguous excerpt from the exact current value of that named field. For a list field, finding_evidence must occur within one current list item. The finding is not new authority and may not supply replacement prose. If prior_validation_failure is present, review the same authoritative candidate again and do not repeat evidence that code proved absent. Do not redesign the product, choose files, paths, tools, dependencies, order, or claim execution status.",
		"Return exactly {\"decision\":\"accept\"} when the specification passes. Otherwise return exactly {\"decision\":\"repair\",\"field\":<one allowed field>,\"finding\":<one concise diagnostic finding>,\"finding_evidence\":<exact excerpt from that current field>}.",
		"APPLICATION_JOB_SPECIFICATION_REVIEW_AUTHORITY_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application job specification review prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationJobSpecificationReviewResponseSchema(
	input ApplicationJobSpecificationReviewInput,
) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	accept := objectSchema(
		[]string{"decision"},
		map[string]any{
			"decision": map[string]any{
				"type": "string", "const": string(ApplicationJobSpecificationReviewAccept),
			},
		},
	)
	repair := objectSchema(
		[]string{"decision", "field", "finding", "finding_evidence"},
		map[string]any{
			"decision": map[string]any{
				"type": "string", "const": string(ApplicationJobSpecificationReviewRepair),
			},
			"field": enumSchema(
				ApplicationJobSpecificationObjectiveField,
				ApplicationJobSpecificationRequiredBehaviorsField,
				ApplicationJobSpecificationAcceptanceCriteriaField,
			),
			"finding": map[string]any{
				"type": "string", "minLength": 1,
				"maxLength": maxApplicationJobSpecificationReviewFindingRunes,
			},
			"finding_evidence": map[string]any{
				"type": "string", "minLength": 1,
				"maxLength": maxApplicationJobSpecificationReviewFindingEvidenceRunes,
			},
		},
	)
	return map[string]any{
		"type":  "object",
		"oneOf": []any{accept, repair},
	}, nil
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
	if wire.Field != nil {
		review.Field = *wire.Field
	}
	if wire.Finding != nil {
		review.Finding = *wire.Finding
	}
	if wire.FindingEvidence != nil {
		review.FindingEvidence = *wire.FindingEvidence
	}
	if err := validateApplicationJobSpecificationReview(review); err != nil {
		return zero, err
	}
	binding, err := applicationJobSpecificationBinding(input.authority, input.retained)
	if err != nil {
		return zero, err
	}
	if review.Decision == ApplicationJobSpecificationReviewRepair {
		observedValueSHA256, err := applicationJobSpecificationCurrentFieldSHA256(
			input.retained, review.Field,
		)
		if err != nil {
			return zero, err
		}
		review.observedValueSHA256 = observedValueSHA256
		if !applicationJobSpecificationReviewEvidenceApplies(input.retained, review) {
			return zero, &ApplicationJobSpecificationReviewEvidenceError{
				Field: review.Field, FindingEvidence: review.FindingEvidence,
				ObservedValueSHA256: observedValueSHA256, RetainedAuthoritySHA256: binding,
			}
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
		if !isApplicationJobSpecificationField(review.Field) {
			return fmt.Errorf("application job specification review field %q is unsupported", review.Field)
		}
		if err := validateApplicationWorkloadLine(
			"application job specification review finding",
			review.Finding,
			maxApplicationJobSpecificationReviewFindingRunes,
		); err != nil {
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

func applicationJobSpecificationReviewEvidenceApplies(
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
) bool {
	switch review.Field {
	case ApplicationJobSpecificationObjectiveField:
		return strings.Contains(retained.Objective, review.FindingEvidence)
	case ApplicationJobSpecificationRequiredBehaviorsField:
		for _, behavior := range retained.RequiredBehaviors {
			if strings.Contains(behavior, review.FindingEvidence) {
				return true
			}
		}
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		for _, criterion := range retained.AcceptanceCriteria {
			if strings.Contains(criterion, review.FindingEvidence) {
				return true
			}
		}
	}
	return false
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
	if err := validateApplicationWorkloadLine(
		"application job specification review failed finding evidence",
		failure.FindingEvidence,
		maxApplicationJobSpecificationReviewFindingEvidenceRunes,
	); err != nil {
		return err
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
	if applicationJobSpecificationReviewEvidenceApplies(retained, ApplicationJobSpecificationReview{
		Field: failure.Field, FindingEvidence: failure.FindingEvidence,
	}) {
		return fmt.Errorf("application job specification review evidence failure now applies to current named field")
	}
	return nil
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
