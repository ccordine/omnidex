package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxApplicationJobSpecificationReviewAttempts = 3
	maxApplicationJobSpecificationRepairAttempts = 2
)

type ApplicationJobSpecificationReviewDecision string

const (
	ApplicationJobSpecificationReviewAccept ApplicationJobSpecificationReviewDecision = "accept"
	ApplicationJobSpecificationReviewRepair ApplicationJobSpecificationReviewDecision = "repair"
)

type ApplicationJobSpecificationReview struct {
	Decision ApplicationJobSpecificationReviewDecision `json:"decision"`
	Field    ApplicationJobSpecificationField          `json:"field,omitempty"`
	Defect   string                                    `json:"defect,omitempty"`

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
	Defect   *string                                    `json:"defect"`
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
	if input.attempt < 1 || input.attempt > maxApplicationJobSpecificationReviewAttempts {
		return fmt.Errorf(
			"application job specification review attempt must be between 1 and %d",
			maxApplicationJobSpecificationReviewAttempts,
		)
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
		Authority     applicationJobSpecificationAuthority `json:"authority"`
		Specification ApplicationJobSpecification          `json:"specification"`
	}{
		Authority:     projectApplicationJobSpecificationAuthority(input.authority),
		Specification: input.retained,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification review authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Review only whether the proposed local job is faithful to the focused accepted requirement and specific enough for a competent developer to implement and verify from this packet.",
		"Accept only when the objective names concrete work rather than merely repeating a noun or saying it works or is usable, every required behavior names a concrete action and result, and every required behavior is covered by a specific observable acceptance criterion. The fields must form one executable job without unrelated scope. Otherwise choose repair and name exactly one target field plus one precise defect. Do not redesign the product, choose files, paths, tools, dependencies, order, or claim execution status.",
		"Return exactly {\"decision\":\"accept\"} when the specification passes. Otherwise return exactly {\"decision\":\"repair\",\"field\":<one allowed field>,\"defect\":<one precise non-empty line>}.",
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
		[]string{"decision", "field", "defect"},
		map[string]any{
			"decision": map[string]any{
				"type": "string", "const": string(ApplicationJobSpecificationReviewRepair),
			},
			"field": enumSchema(
				ApplicationJobSpecificationObjectiveField,
				ApplicationJobSpecificationRequiredBehaviorsField,
				ApplicationJobSpecificationAcceptanceCriteriaField,
			),
			"defect": applicationJobSpecificationLineSchema(maxApplicationWorkloadDefectBytes),
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
	if wire.Defect != nil {
		review.Defect = *wire.Defect
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
		if review.Field != "" || review.Defect != "" {
			return fmt.Errorf("accepted application job specification review must not name a field or defect")
		}
		return nil
	case ApplicationJobSpecificationReviewRepair:
		if !isApplicationJobSpecificationField(review.Field) {
			return fmt.Errorf("application job specification review field %q is unsupported", review.Field)
		}
		if err := validateApplicationWorkloadLine(
			"application job specification review defect", review.Defect, maxApplicationWorkloadDefectBytes,
		); err != nil {
			return err
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
