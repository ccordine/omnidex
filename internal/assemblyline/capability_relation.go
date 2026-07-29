package assemblyline

import (
	"fmt"
	"strings"
)

const CapabilityRelationSchemaV1 = "omnidex.capability-relation.v1"

type CapabilityRelation string

const (
	CapabilityIndependent    CapabilityRelation = "independent"
	CapabilityLeftReadsRight CapabilityRelation = "left_reads_right"
	CapabilityRightReadsLeft CapabilityRelation = "right_reads_left"
	CapabilityBidirectional  CapabilityRelation = "bidirectional"
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
	if len(input.LocalContext) > maxSkillLocalContext ||
		len(input.LeftNeed) > maxSkillProcedureNeed || len(input.RightNeed) > maxSkillProcedureNeed {
		return fmt.Errorf("capability relation input exceeds its hard context limit")
	}
	if input.LeftNeed == input.RightNeed {
		return fmt.Errorf("capability relation requires two distinct local needs")
	}
	return nil
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
		CapabilityRightReadsLeft, CapabilityBidirectional:
		return nil
	default:
		return fmt.Errorf("capability relation %q is unsupported", decision.Relation)
	}
}

func BuildCapabilityRelationPrompt(input CapabilityRelationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the direct live-state dependency between two local behaviors.",
		"A side reads the other only when implementing its own behavior requires current data produced by the other. Shared topic, visual proximity, or possible convenience is not a dependency.",
		"LOCAL_CONTEXT:\n" + input.LocalContext,
		"LEFT_NEED:\n" + input.LeftNeed,
		"RIGHT_NEED:\n" + input.RightNeed,
	}, "\n\n"), nil
}

func CapabilityRelationResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "relation"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": CapabilityRelationSchemaV1},
			"relation": enumSchema(
				CapabilityIndependent, CapabilityLeftReadsRight,
				CapabilityRightReadsLeft, CapabilityBidirectional,
			),
		},
	)
}
