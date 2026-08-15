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
)

type ApplicationJobSpecificationReview struct {
	Decision ApplicationJobSpecificationReviewDecision `json:"decision"`
	Field    ApplicationJobSpecificationField          `json:"field,omitempty"`

	binding string
}

type ApplicationJobSpecificationReviewInput struct {
	authority ApplicationJobSpecificationInput
	retained  ApplicationJobSpecification
	attempt   int
}

type applicationJobSpecificationReviewWire struct {
	Decision *ApplicationJobSpecificationReviewDecision `json:"decision"`
	Field    *ApplicationJobSpecificationField          `json:"field"`
}

func NewApplicationJobSpecificationReviewInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	attempt int,
) (ApplicationJobSpecificationReviewInput, error) {
	input := ApplicationJobSpecificationReviewInput{
		authority: authority, retained: cloneApplicationJobSpecification(retained), attempt: attempt,
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
	return nil
}

func BuildApplicationJobSpecificationReviewPrompt(
	input ApplicationJobSpecificationReviewInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := struct {
		UserAuthority    applicationJobSpecificationAuthority `json:"user_authority"`
		DerivedCandidate ApplicationJobSpecification          `json:"derived_candidate"`
	}{
		UserAuthority:    projectApplicationJobSpecificationAuthority(input.authority),
		DerivedCandidate: input.retained,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification review authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Review only whether derived_candidate is faithful to the focused user_authority and specific enough for a competent developer to implement and verify from this packet.",
		"Accept when the objective names concrete work, every required behavior names a concrete action and result, and observable acceptance criteria collectively cover those behaviors. Qualitative observable results are valid; do not demand invented numeric precision. Reject unrelated scope or capabilities, quantities, counts, ranges, timing, defaults, compatibility promises, or constraints absent from user_authority. A repair decision may identify exactly one derived field; it may not supply replacement prose or new authority. Do not redesign the product, choose files, paths, tools, dependencies, order, or claim execution status.",
		"Return exactly {\"decision\":\"accept\"} when the specification passes. Otherwise return exactly {\"decision\":\"repair\",\"field\":<one allowed field>}.",
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
		[]string{"decision", "field"},
		map[string]any{
			"decision": map[string]any{
				"type": "string", "const": string(ApplicationJobSpecificationReviewRepair),
			},
			"field": enumSchema(
				ApplicationJobSpecificationObjectiveField,
				ApplicationJobSpecificationRequiredBehaviorsField,
				ApplicationJobSpecificationAcceptanceCriteriaField,
			),
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
	if err := validateApplicationJobSpecificationReview(review); err != nil {
		return zero, err
	}
	binding, err := applicationJobSpecificationBinding(input.authority, input.retained)
	if err != nil {
		return zero, err
	}
	review.binding = binding
	return review, nil
}

func validateApplicationJobSpecificationReview(review ApplicationJobSpecificationReview) error {
	switch review.Decision {
	case ApplicationJobSpecificationReviewAccept:
		if review.Field != "" {
			return fmt.Errorf("accepted application job specification review must not name a field")
		}
		return nil
	case ApplicationJobSpecificationReviewRepair:
		if !isApplicationJobSpecificationField(review.Field) {
			return fmt.Errorf("application job specification review field %q is unsupported", review.Field)
		}
		return nil
	default:
		return fmt.Errorf("application job specification review decision %q is unsupported", review.Decision)
	}
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
