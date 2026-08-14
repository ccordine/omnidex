package assemblyline

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type ApplicationJobSpecificationRepairInput struct {
	authority ApplicationJobSpecificationInput
	retained  ApplicationJobSpecification
	review    ApplicationJobSpecificationReview
	attempt   int
}

type ApplicationJobSpecificationRepairPatch struct {
	field      ApplicationJobSpecificationField
	objective  string
	stringList []string
}

type applicationJobSpecificationRepairWire struct {
	Objective          *string   `json:"objective"`
	RequiredBehaviors  *[]string `json:"required_behaviors"`
	AcceptanceCriteria *[]string `json:"acceptance_criteria"`
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
	if input.attempt < 1 || input.attempt > maxApplicationJobSpecificationRepairAttempts {
		return fmt.Errorf(
			"application job specification repair attempt must be between 1 and %d",
			maxApplicationJobSpecificationRepairAttempts,
		)
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
		UserAuthority      applicationJobSpecificationAuthority `json:"user_authority"`
		DerivedCandidate   ApplicationJobSpecification          `json:"derived_candidate"`
		TargetDerivedField ApplicationJobSpecificationField     `json:"target_derived_field"`
	}{
		UserAuthority:    projectApplicationJobSpecificationAuthority(input.authority),
		DerivedCandidate: input.retained, TargetDerivedField: input.review.Field,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification repair authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Repair exactly target_derived_field using the code-owned instruction: " + applicationJobSpecificationRepairInstruction(input.review.Field),
		"Only user_authority contains stated requirements. derived_candidate contains derived build decisions, not user facts. Preserve every other derived field. Use the minimum sufficient concrete detail. Observable does not mean numeric. Do not add capabilities, quantities, counts, ranges, timing, defaults, compatibility promises, or constraints absent from user_authority. Return only the one-field JSON replacement.",
		"APPLICATION_JOB_SPECIFICATION_REPAIR_AUTHORITY_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application job specification repair prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func applicationJobSpecificationRepairInstruction(field ApplicationJobSpecificationField) string {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return "state one concrete local implementation outcome faithful to the focused requirement"
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return "state the minimum concrete user actions and observable results needed to deliver the focused requirement"
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return "state observable checks that collectively cover the retained required behaviors without inventing precision"
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
	definition, err := applicationJobSpecificationFieldSchema(input.review.Field)
	if err != nil {
		return nil, err
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
	var wire applicationJobSpecificationRepairWire
	if err := decodePortablePayload([]byte(raw), &wire); err != nil {
		return zero, fmt.Errorf("decode application job specification repair: %w", err)
	}
	patch := ApplicationJobSpecificationRepairPatch{field: input.review.Field}
	switch input.review.Field {
	case ApplicationJobSpecificationObjectiveField:
		if wire.Objective == nil || wire.RequiredBehaviors != nil || wire.AcceptanceCriteria != nil {
			return zero, fmt.Errorf("application job specification repair must replace only objective")
		}
		if err := validateApplicationWorkloadLine(
			"objective repair", *wire.Objective, maxApplicationObjectiveRunes,
		); err != nil {
			return zero, err
		}
		patch.objective = *wire.Objective
	case ApplicationJobSpecificationRequiredBehaviorsField:
		if wire.RequiredBehaviors == nil || wire.Objective != nil || wire.AcceptanceCriteria != nil {
			return zero, fmt.Errorf("application job specification repair must replace only required_behaviors")
		}
		if err := validateApplicationJobSpecificationList(
			"required behavior", *wire.RequiredBehaviors,
			maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
		); err != nil {
			return zero, err
		}
		patch.stringList = append([]string(nil), (*wire.RequiredBehaviors)...)
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		if wire.AcceptanceCriteria == nil || wire.Objective != nil || wire.RequiredBehaviors != nil {
			return zero, fmt.Errorf("application job specification repair must replace only acceptance_criteria")
		}
		if err := validateApplicationJobSpecificationList(
			"acceptance criterion", *wire.AcceptanceCriteria,
			maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
		); err != nil {
			return zero, err
		}
		patch.stringList = append([]string(nil), (*wire.AcceptanceCriteria)...)
	default:
		return zero, fmt.Errorf("application job specification repair field %q is unsupported", input.review.Field)
	}
	if applicationJobSpecificationRepairIsNoOp(input.retained, patch) {
		return zero, fmt.Errorf("application job specification repair is a no-op")
	}
	return patch, nil
}

func ApplyApplicationJobSpecificationRepair(
	input ApplicationJobSpecificationRepairInput,
	retained ApplicationJobSpecification,
	patch ApplicationJobSpecificationRepairPatch,
) (ApplicationJobSpecification, error) {
	if err := input.validate(); err != nil {
		return ApplicationJobSpecification{}, err
	}
	if !reflect.DeepEqual(retained, input.retained) {
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification repair retained authority drifted",
		)
	}
	if patch.field != input.review.Field || applicationJobSpecificationRepairIsNoOp(retained, patch) {
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification repair patch retargeted immutable authority",
		)
	}
	updated := cloneApplicationJobSpecification(retained)
	switch patch.field {
	case ApplicationJobSpecificationObjectiveField:
		updated.Objective = patch.objective
	case ApplicationJobSpecificationRequiredBehaviorsField:
		updated.RequiredBehaviors = append([]string(nil), patch.stringList...)
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		updated.AcceptanceCriteria = append([]string(nil), patch.stringList...)
	default:
		return ApplicationJobSpecification{}, fmt.Errorf(
			"application job specification repair field %q is unsupported", patch.field,
		)
	}
	if err := ValidateApplicationJobSpecification(updated); err != nil {
		return ApplicationJobSpecification{}, err
	}
	return updated, nil
}

func applicationJobSpecificationFieldSchema(
	field ApplicationJobSpecificationField,
) (map[string]any, error) {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		return applicationJobSpecificationLineSchema(maxApplicationObjectiveRunes), nil
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return applicationJobSpecificationListSchema(maxApplicationBehaviorRunes), nil
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return applicationJobSpecificationListSchema(maxApplicationCriterionRunes), nil
	default:
		return nil, fmt.Errorf("application job specification field %q is unsupported", field)
	}
}

func applicationJobSpecificationRepairIsNoOp(
	retained ApplicationJobSpecification,
	patch ApplicationJobSpecificationRepairPatch,
) bool {
	switch patch.field {
	case ApplicationJobSpecificationObjectiveField:
		return retained.Objective == patch.objective
	case ApplicationJobSpecificationRequiredBehaviorsField:
		return reflect.DeepEqual(retained.RequiredBehaviors, patch.stringList)
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		return reflect.DeepEqual(retained.AcceptanceCriteria, patch.stringList)
	default:
		return true
	}
}
