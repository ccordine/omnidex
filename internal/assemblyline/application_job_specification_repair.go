package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ApplicationJobSpecificationRepairInput struct {
	authority ApplicationJobSpecificationInput
	retained  ApplicationJobSpecification
	review    ApplicationJobSpecificationReview
	attempt   int
}

type ApplicationJobSpecificationRepairPatch struct {
	field       ApplicationJobSpecificationField
	current     string
	replacement string
}

type applicationJobSpecificationRepairAuthority struct {
	Surface            ApplicationSurface `json:"surface"`
	FocusedRequirement string             `json:"focused_requirement"`
}

func NewApplicationJobSpecificationRepairInput(
	authority ApplicationJobSpecificationInput,
	retained ApplicationJobSpecification,
	review ApplicationJobSpecificationReview,
	attempt int,
) (ApplicationJobSpecificationRepairInput, error) {
	input := ApplicationJobSpecificationRepairInput{
		authority: authority, retained: cloneApplicationJobSpecification(retained),
		review: review, attempt: attempt,
	}
	if err := input.validate(); err != nil {
		return ApplicationJobSpecificationRepairInput{}, err
	}
	return input, nil
}

func (input ApplicationJobSpecificationRepairInput) validate() error {
	if err := validateApplicationJobSpecificationBoundReview(
		input.authority, input.retained, input.review,
	); err != nil {
		return err
	}
	if input.review.Resolution != ApplicationJobSpecificationReviewReplace {
		return fmt.Errorf("application job specification repair requires replace resolution")
	}
	if input.attempt < 1 {
		return fmt.Errorf("application job specification repair attempt must be positive")
	}
	return nil
}

func BuildApplicationJobSpecificationRepairPrompt(
	input ApplicationJobSpecificationRepairInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := struct {
		Authority    applicationJobSpecificationRepairAuthority `json:"authority"`
		CurrentField ApplicationJobSpecificationField           `json:"current_field"`
		CurrentValue string                                     `json:"current_value"`
		Problem      string                                     `json:"problem"`
	}{
		Authority: applicationJobSpecificationRepairAuthority{
			Surface:            input.authority.Surface,
			FocusedRequirement: input.authority.FocusedRequirement.SourceQuote,
		},
		CurrentField: input.review.Field,
		CurrentValue: input.review.FindingEvidence,
		Problem:      input.review.Finding,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification repair authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Return one replacement string for current_value that resolves problem under authority.",
		"The response is one JSON object containing only current_field.",
		"APPLICATION_JOB_SPECIFICATION_REPAIR_INPUT_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application job specification repair prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func applicationJobSpecificationCurrentFieldValue(
	retained ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
) (any, error) {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return retained.Objective, nil
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return append([]string(nil), retained.RequiredBehaviors...), nil
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return append([]string(nil), retained.AcceptanceCriteria...), nil
	default:
		return nil, fmt.Errorf("application job specification field %q is unsupported", field)
	}
}

func applicationJobSpecificationRepairInstruction(field ApplicationJobSpecificationField) string {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return "state one concrete local product outcome faithful to the focused requirement"
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return "state the minimum concrete user actions and observable results needed to deliver the focused requirement"
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return "state observable checks that verify the focused requirement without inventing precision"
	default:
		return "reject the unsupported repair target"
	}
}

func ApplicationJobSpecificationRepairResponseSchema(
	input ApplicationJobSpecificationRepairInput,
) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	maximum := maxApplicationObjectiveRunes
	if input.review.Field == ApplicationJobSpecificationRequiredBehaviorsField {
		maximum = maxApplicationBehaviorRunes
	} else if input.review.Field == ApplicationJobSpecificationAcceptanceCriteriaField {
		maximum = maxApplicationCriterionRunes
	}
	field := string(input.review.Field)
	return objectSchema(
		[]string{field},
		map[string]any{field: applicationJobSpecificationLineSchema(maximum)},
	), nil
}

func DecodeApplicationJobSpecificationRepair(
	input ApplicationJobSpecificationRepairInput,
	raw string,
) (ApplicationJobSpecificationRepairPatch, error) {
	var zero ApplicationJobSpecificationRepairPatch
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application job specification repair exceeds %d bytes", maxPortableCandidateBytes)
	}
	wire, err := decodeJSONObject(raw, "application job specification repair")
	if err != nil {
		return zero, err
	}
	field := string(input.review.Field)
	if len(wire) != 1 {
		return zero, fmt.Errorf("application job specification repair must contain exactly one field")
	}
	value, exists := wire[field]
	if !exists {
		return zero, fmt.Errorf("application job specification repair must replace only %s", field)
	}
	patch := ApplicationJobSpecificationRepairPatch{
		field: input.review.Field, current: input.review.FindingEvidence,
	}
	replacement, ok := value.(string)
	if !ok {
		return zero, fmt.Errorf("application job specification repair value must be a string")
	}
	maximum := maxApplicationObjectiveRunes
	label := "objective repair"
	if input.review.Field == ApplicationJobSpecificationRequiredBehaviorsField {
		maximum = maxApplicationBehaviorRunes
		label = "required behavior repair"
	} else if input.review.Field == ApplicationJobSpecificationAcceptanceCriteriaField {
		maximum = maxApplicationCriterionRunes
		label = "acceptance criterion repair"
	}
	if err := validateApplicationWorkloadLine(label, replacement, maximum); err != nil {
		return zero, err
	}
	patch.replacement = replacement
	if applicationJobSpecificationRepairIsNoOp(input.retained, patch) {
		return zero, newApplicationJobSpecificationRepairNoOpError(input.review)
	}
	return patch, nil
}
