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
	remove      bool
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
	if err := validateApplicationJobSpecificationInput(input.authority); err != nil {
		return err
	}
	if err := ValidateApplicationJobSpecification(input.retained); err != nil {
		return fmt.Errorf("application job specification repair requires valid retained state: %w", err)
	}
	if err := validateApplicationJobSpecificationReview(input.review); err != nil {
		return err
	}
	if input.review.Decision != ApplicationJobSpecificationReviewRepair {
		return fmt.Errorf("application job specification repair requires a repair review")
	}
	observedValueSHA256, err := applicationJobSpecificationCurrentFieldSHA256(
		input.retained, input.review.Field,
	)
	if err != nil {
		return err
	}
	if input.review.observedValueSHA256 != observedValueSHA256 {
		return fmt.Errorf("application job specification repair review is not bound to current named field")
	}
	if !applicationJobSpecificationReviewEvidenceApplies(input.retained, input.review) {
		return fmt.Errorf("application job specification repair review evidence does not apply to current named field")
	}
	if input.attempt < 1 {
		return fmt.Errorf("application job specification repair attempt must be positive")
	}
	binding, err := applicationJobSpecificationBinding(input.authority, input.retained)
	if err != nil {
		return err
	}
	if input.review.binding != binding {
		return fmt.Errorf("application job specification repair review is not bound to retained authority")
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
	request := "Return one replacement string for current_value that resolves problem under authority."
	if applicationJobSpecificationRepairCanRemove(input) {
		request = "Return null when current_value is not required by authority.focused_requirement; otherwise return one replacement string that resolves problem."
	}
	prompt := strings.Join([]string{
		request,
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
	definition := any(applicationJobSpecificationLineSchema(maximum))
	if applicationJobSpecificationRepairCanRemove(input) {
		definition = map[string]any{"oneOf": []any{
			applicationJobSpecificationLineSchema(maximum),
			map[string]any{"type": "null"},
		}}
	}
	field := string(input.review.Field)
	return objectSchema([]string{field}, map[string]any{field: definition}), nil
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
	if value == nil {
		if !applicationJobSpecificationRepairCanRemove(input) {
			return zero, fmt.Errorf("application job specification repair cannot remove the final required value")
		}
		patch.remove = true
	} else {
		replacement, ok := value.(string)
		if !ok {
			return zero, fmt.Errorf("application job specification repair value must be a string or null")
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
	}
	if applicationJobSpecificationRepairIsNoOp(input.retained, patch) {
		return zero, newApplicationJobSpecificationRepairNoOpError(input.review)
	}
	return patch, nil
}
