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
	authority := projectApplicationJobSpecificationAuthority(input)
	raw, err := json.Marshal(authority)
	if err != nil {
		return "", fmt.Errorf("encode application job specification authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Turn the focused accepted requirement into one independently executable local job specification.",
		"The objective must say specifically what to implement in the named product; it must not merely repeat the requirement noun or say that it works or is usable. List 1 to 4 required_behaviors that each name a concrete action and result, and 1 to 4 specific observable acceptance_criteria that collectively cover every required behavior. Remain faithful to the focused requirement and use the other accepted requirements only to preserve product meaning and boundaries.",
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
		return applicationJobSpecificationDefect(ApplicationJobSpecificationObjectiveField, err)
	}
	if err := validateApplicationJobSpecificationList(
		"required behavior", specification.RequiredBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
	); err != nil {
		return applicationJobSpecificationDefect(ApplicationJobSpecificationRequiredBehaviorsField, err)
	}
	if err := validateApplicationJobSpecificationList(
		"acceptance criterion", specification.AcceptanceCriteria,
		maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
	); err != nil {
		return applicationJobSpecificationDefect(ApplicationJobSpecificationAcceptanceCriteriaField, err)
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

func applicationJobSpecificationDefect(
	field ApplicationJobSpecificationField,
	err error,
) *ApplicationJobSpecificationDefect {
	return &ApplicationJobSpecificationDefect{Field: field, Detail: err.Error()}
}
