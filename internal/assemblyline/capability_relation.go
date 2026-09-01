package assemblyline

import (
	"fmt"
	"strings"
)

const (
	CapabilityRelationSchemaV1   = "omnidex.capability-relation.v1"
	maxCapabilityRelationContext = 12288
	maxCapabilityRelationNeed    = 2000
)

type CapabilityRelation string

const (
	CapabilityIndependent    CapabilityRelation = "independent"
	CapabilityLeftReadsRight CapabilityRelation = "left_reads_right"
	CapabilityRightReadsLeft CapabilityRelation = "right_reads_left"
)

type CapabilityRelationInput struct {
	LocalContext string `json:"local_context"`
	LeftNeed     string `json:"left_need"`
	RightNeed    string `json:"right_need"`
}

type CapabilityRelationDecision struct {
	Schema   string             `json:"schema"`
	Relation CapabilityRelation `json:"relation"`
}

func (input CapabilityRelationInput) validate() error {
	for label, value := range map[string]string{
		"local context": input.LocalContext,
		"left need":     input.LeftNeed,
		"right need":    input.RightNeed,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("capability relation requires one trimmed %s", label)
		}
	}
	if len(input.LocalContext) > maxCapabilityRelationContext ||
		len(input.LeftNeed) > maxCapabilityRelationNeed || len(input.RightNeed) > maxCapabilityRelationNeed {
		return fmt.Errorf("capability relation input exceeds its hard context limit")
	}
	if input.LeftNeed == input.RightNeed {
		return fmt.Errorf("capability relation requires two distinct local needs")
	}
	return ValidatePathFreeModelContext(
		"capability relation", input.LocalContext, input.LeftNeed, input.RightNeed,
	)
}

func (decision CapabilityRelationDecision) ValidateFor(input CapabilityRelationInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != CapabilityRelationSchemaV1 {
		return fmt.Errorf("capability relation schema must be %q", CapabilityRelationSchemaV1)
	}
	switch decision.Relation {
	case CapabilityIndependent, CapabilityLeftReadsRight,
		CapabilityRightReadsLeft:
		return nil
	default:
		return fmt.Errorf("capability relation %q is unsupported", decision.Relation)
	}
}

func BuildCapabilityRelationPrompt(input CapabilityRelationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := capabilityRelationOpaqueChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"What direct live-state dependency exists between these two local behaviors?",
		[]string{
			"Local context:\n" + input.LocalContext,
			"Left behavior:\n" + input.LeftNeed,
			"Right behavior:\n" + input.RightNeed,
		},
		choices,
	)
}

func DecodeCapabilityRelationDecision(
	input CapabilityRelationInput,
	raw string,
) (CapabilityRelationDecision, error) {
	if err := input.validate(); err != nil {
		return CapabilityRelationDecision{}, err
	}
	choices, err := capabilityRelationOpaqueChoices()
	if err != nil {
		return CapabilityRelationDecision{}, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return CapabilityRelationDecision{}, err
	}
	decision := CapabilityRelationDecision{
		Schema: CapabilityRelationSchemaV1, Relation: CapabilityRelation(leaf),
	}
	if err := decision.ValidateFor(input); err != nil {
		return CapabilityRelationDecision{}, err
	}
	return decision, nil
}

func capabilityRelationOpaqueChoices() ([]OpaqueModelChoice, error) {
	definitions := []struct {
		description string
		value       CapabilityRelation
	}{
		{
			description: "Neither behavior must consume a result uniquely produced by the other. Shared inputs, related subject matter, validation overlap, visual proximity, possible reuse, and convenience do not create a dependency.",
			value:       CapabilityIndependent,
		},
		{
			description: "The left behavior cannot satisfy its stated need without consuming a result uniquely produced by the right behavior.",
			value:       CapabilityLeftReadsRight,
		},
		{
			description: "The right behavior cannot satisfy its stated need without consuming a result uniquely produced by the left behavior.",
			value:       CapabilityRightReadsLeft,
		},
	}
	choices := make([]OpaqueModelChoice, 0, len(definitions))
	for _, definition := range definitions {
		choice, err := NewOpaqueModelChoice(
			definition.description,
			string(definition.value),
		)
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}
