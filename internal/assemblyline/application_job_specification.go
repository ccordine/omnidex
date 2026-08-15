package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type applicationJobSpecificationAuthority struct {
	Surface              ApplicationSurface `json:"surface"`
	ProductQuote         string             `json:"product_quote"`
	AcceptedRequirements []string           `json:"accepted_requirements"`
	FocusedRequirement   string             `json:"focused_requirement"`
}

type applicationJobSpecificationWire struct {
	Objective          *string   `json:"objective"`
	RequiredBehaviors  *[]string `json:"required_behaviors"`
	AcceptanceCriteria *[]string `json:"acceptance_criteria"`
}

func validateApplicationJobSpecificationInput(input ApplicationJobSpecificationInput) error {
	if err := validateApplicationAuthority(
		"application job specification", input.Surface, input.ProductQuote, input.AcceptedRequirements,
	); err != nil {
		return err
	}
	focused := -1
	for index, requirement := range input.AcceptedRequirements {
		if requirement.ID == input.FocusedRequirement.ID {
			focused = index
			break
		}
	}
	if focused < 0 {
		return fmt.Errorf("application job specification focused requirement is not accepted")
	}
	if input.FocusedRequirement != input.AcceptedRequirements[focused] {
		return fmt.Errorf("application job specification focused requirement differs from accepted authority")
	}
	return nil
}

func BuildApplicationJobSpecificationPrompt(input ApplicationJobSpecificationInput) (string, error) {
	if err := validateApplicationJobSpecificationInput(input); err != nil {
		return "", err
	}
	projection := struct {
		UserAuthority applicationJobSpecificationAuthority `json:"user_authority"`
	}{UserAuthority: projectApplicationJobSpecificationAuthority(input)}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application job specification authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Turn the focused accepted requirement into one independently executable local job specification.",
		"Only user_authority contains stated requirements. Your objective, required_behaviors, and acceptance_criteria are minimum sufficient derived build decisions for this build; never present them as user-stated facts.",
		"The objective must say specifically what to implement in the named product; it must not merely repeat the requirement noun or say that it works or is usable. List 1 to 4 required_behaviors that each name a concrete action and result, and 1 to 4 specific observable acceptance_criteria that collectively cover every required behavior. Observable does not require invented numeric precision. Do not add capabilities, quantities, counts, ranges, timing, defaults, compatibility promises, or constraints absent from user_authority. Remain faithful to the focused requirement and use the other accepted requirements only to preserve product meaning and boundaries.",
		"Do not choose files, paths, tools, dependencies, execution order, or claim execution status. Do not add unrelated product scope. Return only the closed JSON response.",
		"APPLICATION_JOB_SPECIFICATION_AUTHORITY_JSON:\n" + string(raw),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application job specification prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func projectApplicationJobSpecificationAuthority(
	input ApplicationJobSpecificationInput,
) applicationJobSpecificationAuthority {
	authority := applicationJobSpecificationAuthority{
		Surface: input.Surface, ProductQuote: input.ProductQuote,
		AcceptedRequirements: make([]string, 0, len(input.AcceptedRequirements)),
		FocusedRequirement:   input.FocusedRequirement.SourceQuote,
	}
	for _, requirement := range input.AcceptedRequirements {
		authority.AcceptedRequirements = append(authority.AcceptedRequirements, requirement.SourceQuote)
	}
	return authority
}

func ApplicationJobSpecificationResponseSchema(
	input ApplicationJobSpecificationInput,
) (map[string]any, error) {
	if err := validateApplicationJobSpecificationInput(input); err != nil {
		return nil, err
	}
	return objectSchema(
		[]string{"objective", "required_behaviors", "acceptance_criteria"},
		map[string]any{
			"objective":           applicationJobSpecificationLineSchema(maxApplicationObjectiveRunes),
			"required_behaviors":  applicationJobSpecificationListSchema(maxApplicationBehaviorRunes),
			"acceptance_criteria": applicationJobSpecificationListSchema(maxApplicationCriterionRunes),
		},
	), nil
}

func DecodeApplicationJobSpecification(
	input ApplicationJobSpecificationInput,
	raw string,
) (ApplicationJobSpecification, error) {
	var zero ApplicationJobSpecification
	if err := validateApplicationJobSpecificationInput(input); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application job specification exceeds %d bytes", maxPortableCandidateBytes)
	}
	if !utf8.ValidString(raw) {
		return zero, fmt.Errorf("application job specification response must be valid UTF-8")
	}
	var wire applicationJobSpecificationWire
	if err := decodePortablePayload([]byte(raw), &wire); err != nil {
		return zero, fmt.Errorf("decode application job specification: %w", err)
	}
	if wire.Objective == nil || wire.RequiredBehaviors == nil || wire.AcceptanceCriteria == nil {
		return zero, fmt.Errorf(
			"application job specification requires objective, required_behaviors, and acceptance_criteria",
		)
	}
	return ApplicationJobSpecification{
		Objective:          *wire.Objective,
		RequiredBehaviors:  append([]string(nil), (*wire.RequiredBehaviors)...),
		AcceptanceCriteria: append([]string(nil), (*wire.AcceptanceCriteria)...),
	}, nil
}

func FirstApplicationJobSpecificationDefect(
	specification ApplicationJobSpecification,
) *ApplicationJobSpecificationDefect {
	if err := validateApplicationWorkloadLine(
		"objective", specification.Objective, maxApplicationObjectiveRunes,
	); err != nil {
		return applicationJobSpecificationDefectAt(
			ApplicationJobSpecificationObjectiveField, "objective", err,
		)
	}
	if defect := firstApplicationJobSpecificationListDefect(
		"required behavior", specification.RequiredBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
		ApplicationJobSpecificationRequiredBehaviorsField,
	); defect != nil {
		return defect
	}
	if defect := firstApplicationJobSpecificationListDefect(
		"acceptance criterion", specification.AcceptanceCriteria,
		maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
		ApplicationJobSpecificationAcceptanceCriteriaField,
	); defect != nil {
		return defect
	}
	return nil
}

func ValidateApplicationJobSpecification(specification ApplicationJobSpecification) error {
	if defect := FirstApplicationJobSpecificationDefect(specification); defect != nil {
		return defect
	}
	return nil
}

func applicationJobSpecificationLineSchema(maximum int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": maximum}
}

func applicationJobSpecificationListSchema(maximum int) map[string]any {
	return map[string]any{
		"type": "array", "minItems": 1, "maxItems": 4,
		"items": applicationJobSpecificationLineSchema(maximum),
	}
}

func firstApplicationJobSpecificationListDefect(
	label string,
	values []string,
	maximumCount int,
	maximumRunes int,
	field ApplicationJobSpecificationField,
) *ApplicationJobSpecificationDefect {
	if len(values) < 1 || len(values) > maximumCount {
		return applicationJobSpecificationDefectAt(
			field, "", fmt.Errorf("requires 1..%d %ss", maximumCount, label),
		)
	}
	seen := make(map[string]int, len(values))
	for index, value := range values {
		target := fmt.Sprintf("%s_%03d", field, index+1)
		if err := validateApplicationWorkloadLine(label, value, maximumRunes); err != nil {
			return applicationJobSpecificationDefectAt(
				field, target, fmt.Errorf("%s %d: %w", label, index, err),
			)
		}
		if earlier, duplicate := seen[value]; duplicate {
			return applicationJobSpecificationDefectAt(
				field, target,
				fmt.Errorf("%s %d duplicates earlier item %d", label, index, earlier),
			)
		}
		seen[value] = index
	}
	return nil
}

func applicationJobSpecificationDefectAt(
	field ApplicationJobSpecificationField,
	target string,
	err error,
) *ApplicationJobSpecificationDefect {
	return &ApplicationJobSpecificationDefect{
		Field: field, Detail: err.Error(), correctionTarget: target,
	}
}
